package publisher

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"

	"github.com/go-resty/resty/v2"
	"github.com/rnr-capital/newsfeed-backend/bot"
	"github.com/rnr-capital/newsfeed-backend/collector"
	"github.com/rnr-capital/newsfeed-backend/model"
	. "github.com/rnr-capital/newsfeed-backend/protocol"
	"github.com/rnr-capital/newsfeed-backend/server/resolver"
	. "github.com/rnr-capital/newsfeed-backend/utils"
	. "github.com/rnr-capital/newsfeed-backend/utils/log"
)

const SemanticHashingLength = 128
const EmbeddingLength = 100

type CrawlerpublisherMessageProcessor struct {
	Reader MessageQueueReader
	DB     *gorm.DB

	// gRPC Client and connection
	Client DeduplicatorClient

	// This map stores all existing dedup id since the processor starts. This is
	// to cache the existing posts by dedup id so that we don't query DB to find
	// out whether a post exists. Note that it can return false negative, meaning
	// that a post might not exist in this map but still exists in the DB.
	// About 4k distinct posts enters DB every day, which is pretty trivial to
	// store in memory.
	m                  sync.RWMutex
	ExistingDedupIdMap map[string]bool
}

// Create new processor with reader dependency injection
func NewPublisherMessageProcessor(
	reader MessageQueueReader,
	db *gorm.DB,
	client DeduplicatorClient,
) *CrawlerpublisherMessageProcessor {
	return &CrawlerpublisherMessageProcessor{
		Reader:             reader,
		DB:                 db,
		Client:             client,
		m:                  sync.RWMutex{},
		ExistingDedupIdMap: make(map[string]bool),
	}
}

// Use Reader to read N messages and process them in parallel
// Time out or queue name etc are defined in reader
// Reader focus on how to get message from queue
// Processor focus on how to process the message
// This function doesn't return anything, only log errors
func (processor *CrawlerpublisherMessageProcessor) ReadAndProcessMessages(sqsReadBatchSize int64) int {
	// Pull queued messages from queue
	msgs, err := processor.Reader.ReceiveMessages(sqsReadBatchSize)

	successCount := 0
	if err != nil {
		LogV2.Error("fail read crawler messages from queue : " + err.Error())
		return successCount
	}

	// TODO: process in parallel, but can involve time ordering issue
	// Process all messages
	for _, msg := range msgs {
		if p, err := processor.ProcessOneCralwerMessage(msg); err != nil {
			LogV2.Error(fmt.Sprintf("fail process one crawler message. err: %s , message: %s, subsource: %s", err, p.Post.Content, p.Post.SubSource.Name))
		} else {
			successCount++
		}
		// TODO: It would be better to move message to DLQ in case of error, instead
		// of just delete it for all cases.
		if processor.Reader.DeleteMessage(msg) != nil {
			LogV2.Error(fmt.Sprintf("fail to delete message from SQS: %s", *msg.Message))
		}

	}
	return successCount
}

func cutToTokens(s string, n int) string {
	words := strings.Fields(s)
	var tokens []string
	tokenCount := 0

	for _, word := range words {
		runes := []rune(word)
		if tokenCount+len(runes) > n {
			remaining := n - tokenCount
			tokens = append(tokens, string(runes[:remaining]))
			break
		} else {
			tokens = append(tokens, word)
			tokenCount += len(runes)
		}
	}

	return strings.Join(tokens, " ")
}

func CalculateEmbedding(text string) ([]float32, error) {
	if len(text) == 0 {
		return nil, nil
	}

	const lambdaUrl = "https://2ksbsep74dkq6x64n2u4nok4am0oqmja.lambda-url.us-west-1.on.aws/"
	client := resty.New()

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(cutToTokens(text, 7000)).
		Post(lambdaUrl)
	if err != nil {
		return nil, err
	}

	var floatArray []float32
	err = json.Unmarshal(resp.Body(), &floatArray)
	if err != nil {
		return nil, err
	}
	return floatArray, nil
}

func (processor *CrawlerpublisherMessageProcessor) calculateSemanticHashing(decodedMsg *CrawlerMessage) (string, error) {
	// We don't calculate semantic hashing for Wechat/Twitter message because their
	// contents might be empty or not meaningful.
	if decodedMsg.Post.SubSource.SourceId == collector.WeixinSourceId ||
		decodedMsg.Post.SubSource.SourceId == collector.TwitterSourceId {
		return "", nil
	}

	// Calculate semanticHashing by calling Deduplicator.
	ctx := context.Background()
	content := decodedMsg.Post.Content
	if content == "" {
		content = decodedMsg.Post.Title + decodedMsg.Post.Content
	}
	res, err := processor.Client.GetSimHash(ctx, &GetSimHashRequest{
		Text:   content,
		Length: SemanticHashingLength,
	})
	if err != nil || len(res.Binary) != SemanticHashingLength {
		// LogV2.Error("fail to calculate the semantic hashing for post: " + decodedMsg.String() + " error: " + err.Error() + "hashing: " + res.Binary)
		return "", err
	}
	return res.Binary, nil
}

// Check whether a post exist in DB by dedup_id. It will firstly go through the
// local dedup_id cache, if not then lookup in DB, populate local cache if
// result is found in DB.
func (processor *CrawlerpublisherMessageProcessor) isPostExist(decodedMsg *CrawlerMessage) bool {
	// First check whether the cache contains the dedup id.
	processor.m.RLock()
	if _, ok := processor.ExistingDedupIdMap[decodedMsg.Post.DeduplicateId]; ok {
		processor.m.RUnlock()
		return true
	}
	processor.m.RUnlock()

	// If not, check the DB
	var post model.Post = model.Post{}
	post.SubSource.SourceID = ""
	res := false
	LogV2.Log(fmt.Sprintf("decoded: dedup_id: %s, source: %s, content: %s, origin_url: %s, %v",
		decodedMsg.Post.DeduplicateId, decodedMsg.Post.SubSource.SourceId, decodedMsg.Post.Content,
		decodedMsg.Post.OriginUrl, decodedMsg))
	if post.SubSource.SourceID == "0129417c-4987-45c9-86ac-d6a5c89fb4f7" {
		res = (processor.DB.Where(
			"deduplicate_id = ? OR origin_url = ?",
			decodedMsg.Post.DeduplicateId, decodedMsg.Post.OriginUrl,
		).First(&post).RowsAffected != 0)
	} else {
		test := (processor.DB.Where(
			"deduplicate_id = ?",
			decodedMsg.Post.DeduplicateId,
		).First(&post))
		res = !errors.Is(test.Error, gorm.ErrRecordNotFound)
	}
	LogV2.Log(fmt.Sprintf("found: dedup_id: %s, id: %s, origin_url: %s, duplicate?: %v, %v",
		post.DeduplicateId, post.Id, post.OriginUrl, res, post))

	// If we found the entry in DB but not in the local cache, we should populate
	// that dedup id in cache.
	if res {
		processor.m.Lock()
		processor.ExistingDedupIdMap[decodedMsg.Post.DeduplicateId] = true
		processor.m.Unlock()
	} else {
		// unique post
		LogV2.Log(fmt.Sprintf("new post %s, origin_url: %s, content: %s", decodedMsg.Post.DeduplicateId, decodedMsg.Post.OriginUrl, decodedMsg.Post.Content))

	}

	return res
}

func (processor *CrawlerpublisherMessageProcessor) prepareFeedCandidates(
	subSource *model.SubSource,
) map[string]*model.Feed {
	feedCandidates := make(map[string]*model.Feed)

	if subSource != nil {
		for _, feed := range subSource.Feeds {
			feedCandidates[feed.Id] = feed
		}
	}
	return feedCandidates
}

func (processor *CrawlerpublisherMessageProcessor) prepareSubSourceRecursive(post *CrawlerMessage_CrawledPost, isRoot bool) (*model.SubSource, error) {
	subSource, err := resolver.UpsertSubsourceImpl(processor.DB, model.UpsertSubSourceInput{
		Name:               post.SubSource.Name,
		ExternalIdentifier: post.SubSource.ExternalId,
		SourceID:           post.SubSource.SourceId,
		AvatarURL:          post.SubSource.AvatarUrl,
		OriginURL:          post.SubSource.OriginUrl,
		IsFromSharedPost:   !isRoot,
	})
	if err != nil {
		return nil, err
	}
	post.SubSource.Id = subSource.Id

	if post.SharedFromCrawledPost != nil {
		if _, err = processor.prepareSubSourceRecursive(post.SharedFromCrawledPost, false); err != nil {
			return nil, err
		}
	}

	if post.ReplyTo != nil {
		if _, err = processor.prepareSubSourceRecursive(post.ReplyTo, false); err != nil {
			return nil, err
		}
	}

	return subSource, nil
}

func (processor *CrawlerpublisherMessageProcessor) prepareReplyThreadFromMessage(
	msg *CrawlerMessage, currentPost *CrawlerMessage_CrawledPost) (
	[]*model.Post, error) {
	res := []*model.Post{}
	post, err := processor.preparePostChainFromMessage(msg, currentPost, false /*isRoot*/)
	if err != nil {
		return nil, err
	}
	res = append(res, post)

	if currentPost.ReplyTo == nil {
		return res, nil
	}

	parents, err := processor.prepareReplyThreadFromMessage(msg, currentPost.ReplyTo)
	if err != nil {
		return nil, err
	}

	res = append(res, parents...)
	return res, nil
}

func (processor *CrawlerpublisherMessageProcessor) preparePostChainFromMessage(msg *CrawlerMessage, currentPost *CrawlerMessage_CrawledPost, isRoot bool) (post *model.Post, e error) {
	var subSource model.SubSource
	res := processor.DB.Where("id = ?", currentPost.SubSource.Id).First(&subSource)
	if res.RowsAffected == 0 {
		return nil, errors.New("invalid subsource id " + currentPost.SubSource.Id)
	}

	post = &model.Post{
		Id:                 uuid.New().String(),
		Title:              currentPost.Title,
		Content:            currentPost.Content,
		CreatedAt:          time.Now(),
		SubSource:          subSource,
		SubSourceID:        currentPost.SubSource.Id,
		ReadByUser:         []*model.User{},
		PublishedFeeds:     []*model.Feed{},
		InSharingChain:     !isRoot,
		DeduplicateId:      currentPost.DeduplicateId,
		CrawledAt:          msg.CrawledAt.AsTime(),
		ContentGeneratedAt: currentPost.ContentGeneratedAt.AsTime(),
		ImageUrls:          currentPost.ImageUrls,
		FileUrls:           currentPost.FilesUrls,
		OriginUrl:          currentPost.OriginUrl,
		// transform tags into serialized string separated by ","
		Tag: strings.Join(currentPost.Tags, ","),
	}
	if currentPost.SharedFromCrawledPost != nil {
		sharedFromPost, e := processor.preparePostChainFromMessage(msg, currentPost.SharedFromCrawledPost, false)
		if e != nil {
			return nil, e
		}
		post.SharedFromPost = sharedFromPost
		post.SharedFromPostID = &sharedFromPost.Id
	}

	// Only construct reply thread at root level
	if isRoot && currentPost.ReplyTo != nil {
		thread, err := processor.prepareReplyThreadFromMessage(msg, currentPost.ReplyTo)
		if err != nil {
			return nil, err
		}
		post.ReplyThread = thread
	}

	return post, nil
}

func (processor *CrawlerpublisherMessageProcessor) MatchMessageWithFeeds(feedCandidates map[string]*model.Feed, post *model.Post) ([]*model.Feed, error) {
	var wg sync.WaitGroup
	ch := make(chan *model.Feed, len(feedCandidates))
	errCh := make(chan error, len(feedCandidates))
	for _, feed := range feedCandidates {
		// since we will append pointer, we need to have a var each iteration
		// otherwise feeds appended will be reused and all feeds in the slice are same
		// feed := feedCandidates[ind]
		// Once a message is matched to a feed, write the PostFeedPublish relation to DB
		wg.Add(1)
		go func(feed *model.Feed) {
			defer wg.Done()
			matched, err := DataExpressionMatchPostChain(feed.FilterDataExpression.String(), post)
			if err != nil {
				errCh <- err
			} else if matched {
				ch <- feed
			}
		}(feed)
	}

	// wait for all goroutines to finish
	wg.Wait()
	close(ch)
	close(errCh)

	feedsToPublish := []*model.Feed{}
	for feed := range ch {
		feedsToPublish = append(feedsToPublish, feed)
	}
	if err, ok := <-errCh; ok {
		return nil, err
	}
	return feedsToPublish, nil
}

// Process one cralwer-publisher message in following major steps:
// Step1. decode into protobuf generated struct
// Step2. update subsource
// Step2. deduplication
// Step3. do publishing with new post, also handle recursive shared_from posts
// Step4. if publishing succeeds, delete message in queue
func (processor *CrawlerpublisherMessageProcessor) ProcessOneCralwerMessage(msg *MessageQueueMessage) (*CrawlerMessage, error) {
	// TODO: bump counter in ddog for number of message processed
	decodedMsg, err := processor.DecodeCrawlerMessage(msg)
	if err != nil {
		return nil, err
	}

	// Once get a message, check if there is exact same Post (same sources, same
	// content), if not store into DB as Post.
	if processor.isPostExist(decodedMsg) {
		// Log.Infof("[duplicated message] message has already been processed, existing deduplicate_id: %s, existing post_id: %s ", decodedMsg.Post.DeduplicateId, existingPost.Id)
		// TODO: bump counter for deduplicated messages
		return decodedMsg, nil
	}

	// Prepare Post relations to Subsources (Sources can be inferred)
	subSource, err := processor.prepareSubSourceRecursive(decodedMsg.Post /*isRoot*/, true)
	if err != nil {
		return decodedMsg, err
	}

	// Load feeds into memory based on source and subsource of the post
	feedCandidates := processor.prepareFeedCandidates(subSource)

	// Create new post based on message
	post, err := processor.preparePostChainFromMessage(
		decodedMsg,
		decodedMsg.Post,
		/*isRoot*/ true,
	)
	if err != nil {
		return decodedMsg, err
	}

	h, err := processor.calculateSemanticHashing(decodedMsg)
	// Only do soft failure for semantic hashing uncalculated. This is because
	// semantic hashing is a "Good to have" feature, App can still work properly
	// without it.
	if err == nil && len(h) == SemanticHashingLength {
		post.SemanticHashing = h
	} else {
		LogV2.Error(fmt.Sprintf("fail to calculate semantic hashing for message: %s, err: %s, hashing: %s", decodedMsg.String(), err, h))
	}

	e, err := CalculateEmbedding(decodedMsg.Post.Title + decodedMsg.Post.Content)
	if err == nil && len(e) == EmbeddingLength {
		vec := pgvector.NewVector(e)
		post.Embedding = &vec
	} else {
		LogV2.Error(fmt.Sprintf("failed to calculate embedding for message: %s. Err: %v", decodedMsg.Post.Title+decodedMsg.Post.Content, err))
		post.Embedding = nil
	}

	if decodedMsg.Post.SharedFromCrawledPost != nil {
		e, err := CalculateEmbedding(decodedMsg.Post.SharedFromCrawledPost.Title + decodedMsg.Post.SharedFromCrawledPost.Content)
		if err == nil && len(e) == EmbeddingLength {
			vec := pgvector.NewVector(e)
			post.SharedFromPost.Embedding = &vec
		} else {
			LogV2.Error(fmt.Sprintf("failed to calculate embedding for message: %s. Err: %s", decodedMsg.Post.Title+decodedMsg.Post.Content, err))
			post.SharedFromPost.Embedding = nil
		}
	}

	// Match post with candidate feeds in parallel
	feedsToPublish, err := processor.MatchMessageWithFeeds(feedCandidates, post)
	if err != nil {
		return decodedMsg, err
	}

	columns := []*model.Column{}
	for _, f := range feedsToPublish {
		cs := f.Columns
		columns = append(columns, cs...)
	}

	// // TODO: skip if there is no subscribers
	go bot.TimeBoundedNotifyPost(context.Background(), *post, columns)

	channelsPushed := map[string]struct{}{}
	for _, f := range feedsToPublish {
		for _, cl := range f.Columns {
			for _, c := range cl.SubscribedChannels {
				if _, ok := channelsPushed[c.Id]; ok {
					continue
				}
				go bot.TimeBoundedPushPost(context.Background(), c.WebhookUrl, *post)
				channelsPushed[c.Id] = struct{}{}
			}
		}
	}

	// // Write to DB, post creation and publish is in a transaction
	err = processor.DB.Transaction(func(tx *gorm.DB) error {
		sql := processor.DB.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return tx.Create(&post)
		})
		if err := tx.Create(&post).Error; err != nil {
			LogV2.Error(fmt.Sprintf("fail to create post. Error %s. %s %s FROM %s. Embedding %v. Raw query: %s", err, post.Content, post.Title, post.SubSource.Name, post.Embedding, sql))
			return err
		} else {
			LogV2.Info(fmt.Sprintf("created post. %s %s FROM %s. Embedding %v. Raw query: %s", post.Content, post.Title, post.SubSource.Name, post.Embedding, sql))
		}

		if err := tx.Model(&post).Association("PublishedFeeds").Append(feedsToPublish); err != nil {
			LogV2.Error(fmt.Sprintf("fail to save post feed relationship. Error %s. %s %s FROM %s", err, post.Content, post.Title, post.SubSource.Name))
			return err
		}

		return nil
	})
	if err != nil {
		return decodedMsg, err
	}

	return decodedMsg, nil
}

// Parse message into meaningful structure CrawlerMessage
// This function assumes message passed in can be parsed, otherwise it will throw error
func (processor *CrawlerpublisherMessageProcessor) DecodeCrawlerMessage(msg *MessageQueueMessage) (*CrawlerMessage, error) {
	str, err := msg.Read()
	if err != nil {
		return nil, err
	}

	sDec, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return nil, err
	}

	decodedMsg := &CrawlerMessage{}
	if err := proto.Unmarshal(sDec, decodedMsg); err != nil {
		return nil, err
	}

	return decodedMsg, nil
}
