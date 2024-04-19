package sink

import (
	"fmt"

	"github.com/rnr-capital/newsfeed-backend/protocol"
	Logger "github.com/rnr-capital/newsfeed-backend/utils/log"
	"google.golang.org/protobuf/encoding/prototext"
)

type StdErrSink struct{}

func NewStdErrSink() *StdErrSink {
	return &StdErrSink{}
}

func (s *StdErrSink) Push(msg *protocol.CrawlerMessage) error {
	if msg == nil {
		return nil
	}
	Logger.LogV2.Info(fmt.Sprint("=== mock pushed to SNS with CrawlerMessage === \n", prototext.Format(msg)))
	return nil
}
