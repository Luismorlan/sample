package panoptic

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/rnr-capital/newsfeed-backend/model"
	"github.com/rnr-capital/newsfeed-backend/protocol"
	"github.com/rnr-capital/newsfeed-backend/utils"
	"github.com/rnr-capital/newsfeed-backend/utils/dotenv"
)

func TestMain(m *testing.M) {
	dotenv.LoadDotEnvsInTests()
	os.Exit(m.Run())
}

func TestMergeSubsourcesFromConfigAndDb(t *testing.T) {
	db, _ := utils.CreateTempDB(t)

	user := model.User{
		Id:   uuid.New().String(),
		Name: "test_user",
	}

	sourceId := uuid.New().String()
	source := model.Source{
		Id:        sourceId,
		Name:      "博客",
		Domain:    "",
		CreatedAt: time.Now(),
		Creator:   user,
	}
	db.Create(&source)

	crawlerParams := `crawl_url:"https://www.cls.cn/telegraph" base_selector:".telegraph-list" title_relative_selector:".telegraph-content-box span:not(.telegraph-time-box) > strong" content_relative_selector:".telegraph-content-box span:not(.telegraph-time-box)" external_id_relative_selector:"" time_relative_selector:"" image_relative_selector:".telegraph-images-box > img" origin_url_relative_selector:""`
	subSource := model.SubSource{
		Id:                      uuid.New().String(),
		Name:                    "博客_from_DB",
		SourceID:                sourceId,
		CreatedAt:               time.Now(),
		CustomizedCrawlerParams: &crawlerParams,
	}
	db.Create(&subSource)

	// Write this to DB, but because it already exists in config, don't overwrite the one in config
	subSource2 := model.SubSource{
		Id:                      uuid.New().String(),
		Name:                    "博客_in_config",
		SourceID:                sourceId,
		CreatedAt:               time.Now(),
		CustomizedCrawlerParams: &crawlerParams,
	}
	db.Create(&subSource2)

	GetCustomizedSubsourceSourceId(db)

	configs := protocol.PanopticConfigs{
		Config: []*protocol.PanopticConfig{
			{
				Name:            "config_1",
				DataCollectorId: protocol.PanopticTask_COLLECTOR_USER_CUSTOMIZED_SUBSOURCE,
				TaskParams: &protocol.TaskParams{
					SourceId: sourceId,
					SubSources: []*protocol.PanopticSubSource{
						{
							Name: "博客_in_config",
						},
					},
				},
			},
		},
	}
	MergeSubsourcesFromConfigAndDb(db, &configs)
	require.Len(t, configs.Config[0].TaskParams.SubSources, 2)
	require.Equal(t, configs.Config[0].TaskParams.SubSources[0].Name, "博客_in_config")
	require.Equal(t, configs.Config[0].TaskParams.SubSources[1].Name, "博客_from_DB")
	require.Equal(t, configs.Config[0].TaskParams.SubSources[1].CustomizedCrawlerParamsForSubSource.BaseSelector, ".telegraph-list")
	require.Equal(t, configs.Config[0].TaskParams.SubSources[1].CustomizedCrawlerParamsForSubSource.CrawlUrl, "https://www.cls.cn/telegraph")
	require.Equal(t, *configs.Config[0].TaskParams.SubSources[1].CustomizedCrawlerParamsForSubSource.TitleRelativeSelector, ".telegraph-content-box span:not(.telegraph-time-box) > strong")
}

func TestMergeSubsourcesFromConfigAndDb_Twitter(t *testing.T) {
	db, _ := utils.CreateTempDB(t)

	user := model.User{
		Id:   uuid.New().String(),
		Name: "test_user",
	}

	twitterSourceId := uuid.New().String()
	twitterSource := model.Source{
		Id:        twitterSourceId,
		Name:      "推特",
		Domain:    "",
		CreatedAt: time.Now(),
		Creator:   user,
	}
	db.Create(&twitterSource)

	twitterSubSourceInDB := model.SubSource{
		Id:               uuid.New().String(),
		Name:             "马斯克",
		SourceID:         twitterSourceId,
		CreatedAt:        time.Now(),
		IsFromSharedPost: false,
	}
	db.Create(&twitterSubSourceInDB)

	// Write this to DB, but because it already exists in config, don't overwrite the one in config
	twitterSubSourceInConfig := model.SubSource{
		Id:        uuid.New().String(),
		Name:      "贝索斯",
		SourceID:  twitterSourceId,
		CreatedAt: time.Now(),
	}
	db.Create(&twitterSubSourceInConfig)

	ids := GetCustomizedSubsourceSourceId(db)
	require.Equal(t, len(ids), 1)
	require.True(t, ids[twitterSourceId])

	configs := protocol.PanopticConfigs{
		Config: []*protocol.PanopticConfig{
			{
				Name:            "twitter_config",
				DataCollectorId: protocol.PanopticTask_COLLECTOR_TWITTER,
				TaskParams: &protocol.TaskParams{
					SourceId: twitterSourceId,
					SubSources: []*protocol.PanopticSubSource{
						{
							Name: "贝索斯",
						},
					},
				},
			},
		},
	}
	MergeSubsourcesFromConfigAndDb(db, &configs)
	require.Len(t, configs.Config[0].TaskParams.SubSources, 2)
	require.Equal(t, configs.Config[0].TaskParams.SubSources[0].Name, "贝索斯")
	require.Equal(t, configs.Config[0].TaskParams.SubSources[1].Name, "马斯克")
}
