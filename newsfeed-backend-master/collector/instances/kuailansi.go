package collector_instances

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/rnr-capital/newsfeed-backend/collector"
	sink "github.com/rnr-capital/newsfeed-backend/collector/sink"
	"github.com/rnr-capital/newsfeed-backend/collector/working_context"
	"github.com/rnr-capital/newsfeed-backend/protocol"
	"github.com/rnr-capital/newsfeed-backend/utils"
	Logger "github.com/rnr-capital/newsfeed-backend/utils/log"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	KuailansiUrl        = "http://m.fbecn.com/24h/news_fbe0406.json?newsid=0"
	KuailansiOriginUrl  = "http://m.fbecn.com/"
	ChinaTimeZone       = "Asia/Shanghai"
	KuailansiTimeFormat = "2006-01-02 15:04:05"

	IpBanMessage = "IP访问受限制"
)

type KuailansiApiCrawler struct {
	Sink sink.CollectedDataSink
}

type KuailansiPost struct {
	NewsId   string `json:"newsID"`
	Time     string `json:"time"`
	Content  string `json:"content"`
	Level    string `json:"Level"`
	Type     string `json:"Type"`
	Keywords string `json:"Keywords"`
}

func (p *KuailansiPost) GetContent() string {
	re := regexp.MustCompile(`【.*】`)
	match := re.FindStringSubmatch(p.Content)
	if len(match) != 1 {
		return p.Content
	}
	return strings.ReplaceAll(p.Content, match[0], "")
}

func (p *KuailansiPost) GetTitle() string {
	re := regexp.MustCompile(`【.*】`)
	match := re.FindStringSubmatch(p.Content)
	if len(match) != 1 {
		return ""
	}
	replacer := strings.NewReplacer("【", "", "】", "")
	return replacer.Replace(match[0])
}

type KuailansiApiResponse struct {
	List     []KuailansiPost `json:"list"`
	NextPage string          `json:"nextpage"`
}

// For kuailansi, if Level == 0, it's a important update.
func (k KuailansiApiCrawler) GetNewsTypeForPost(post *KuailansiPost) (protocol.PanopticSubSource_SubSourceType, error) {
	level, err := strconv.Atoi(post.Level)
	if err != nil {
		return protocol.PanopticSubSource_UNSPECIFIED, errors.Wrap(err, "cannot parse Kuailansi post.Level")
	}

	if level >= 1 {
		return protocol.PanopticSubSource_FLASHNEWS, nil
	}

	return protocol.PanopticSubSource_KEYNEWS, nil
}

func (k KuailansiApiCrawler) GetCrawledSubSourceNameFromPost(post *KuailansiPost) (string, error) {
	t, err := k.GetNewsTypeForPost(post)
	if err != nil {
		return "", errors.Wrap(err, "fail to get subsource type from post"+collector.PrettyPrint(post))
	}
	return collector.SubsourceTypeToName(t), nil
}

func (k KuailansiApiCrawler) ParseGenerateTime(post *KuailansiPost) (*timestamppb.Timestamp, error) {
	location, err := time.LoadLocation(ChinaTimeZone)
	if err != nil {
		return nil, errors.Wrap(err, "fail to parse time zome for Kuailansi: "+ChinaTimeZone)
	}
	t, err := time.ParseInLocation(KuailansiTimeFormat, post.Time, location)
	if err != nil {
		return nil, errors.Wrap(err, "fail to parse Kuailansi post time: "+post.Time)
	}
	return timestamppb.New(t), nil
}

func (k KuailansiApiCrawler) ValidatePost(post *KuailansiPost) error {
	if strings.Contains(post.Content, IpBanMessage) {
		return errors.New("Kuailansi IP is banned")
	}
	return nil
}

func (k KuailansiApiCrawler) ProcessSinglePost(post *KuailansiPost,
	workingContext *working_context.ApiCollectorWorkingContext) error {
	if err := k.ValidatePost(post); err != nil {
		return err
	}

	subSourceType, err := k.GetNewsTypeForPost(post)
	if err != nil {
		return err
	}

	if !collector.IsRequestedNewsType(workingContext.Task.TaskParams.SubSources, subSourceType) {
		// Return nil if the post is not of requested type. Note that this is
		// intentionally not considered as failure.
		return nil
	}

	collector.InitializeApiCollectorResult(workingContext)

	ts, err := collector.ParseGenerateTime(post.Time, KuailansiTimeFormat, ChinaTimeZone, "kuailansi")
	if err != nil {
		return err
	}

	name, err := k.GetCrawledSubSourceNameFromPost(post)
	if err != nil {
		return errors.Wrap(err, "cannot find post subsource")
	}

	workingContext.Result.Post.ContentGeneratedAt = ts
	if post.GetTitle() != "" {
		workingContext.Result.Post.Title = post.GetTitle()
	}
	workingContext.Result.Post.Content = post.GetContent()
	workingContext.Result.Post.SubSource.Name = name
	workingContext.Result.Post.SubSource.AvatarUrl = collector.GetSourceLogoUrl(
		workingContext.Task.TaskParams.SourceId)
	workingContext.Result.Post.SubSource.OriginUrl = KuailansiOriginUrl

	err = k.GetDedupId(workingContext)
	if err != nil {
		return errors.Wrap(err, "cannot get dedup id from post.")
	}

	return nil
}

func (k KuailansiApiCrawler) GetDedupId(workingContext *working_context.ApiCollectorWorkingContext) error {
	md5, err := utils.TextToMd5Hash(workingContext.Result.Post.SubSource.Id + workingContext.Result.Post.Content)
	if err != nil {
		return err
	}
	workingContext.Result.Post.DeduplicateId = md5
	return nil
}

func (k KuailansiApiCrawler) CollectAndPublish(task *protocol.PanopticTask) {
	res := &KuailansiApiResponse{}
	err := collector.HttpGetAndParseJsonResponse(KuailansiUrl, res)
	if err != nil {
		Logger.LogV2.Errorf("fail to get Kuailansi response:", err)
		task.TaskMetadata.ResultState = protocol.TaskMetadata_STATE_FAILURE
		return
	}

	for _, post := range res.List {
		workingContext := &working_context.ApiCollectorWorkingContext{
			SharedContext: working_context.SharedContext{Task: task, IntentionallySkipped: false},
			ApiUrl:        KuailansiUrl,
		}

		err := k.ProcessSinglePost(&post, workingContext)
		if err != nil {
			Logger.LogV2.Errorf("fail to process a single Kuailansi Post:", err,
				"\npost content:\n", collector.PrettyPrint(post))
			workingContext.Task.TaskMetadata.TotalMessageFailed++
			continue
		}

		// Returning nil in ProcessSinglePost doesn't necessarily mean success, it
		// could just be that we're skiping that post (e.g. subsource type doesn't
		// match)
		if workingContext.SharedContext.Result != nil {
			sink.PushResultToSinkAndRecordInTaskMetadata(k.Sink, workingContext)
		}
	}

	collector.SetErrorBasedOnCounts(task, KuailansiUrl)
}
