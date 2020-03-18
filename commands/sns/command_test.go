package sns

import (
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"

	"testing"

	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/habx/aws-mq-cleaner/helpers"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_SNS(t *testing.T) {
	snsLocalStack := helpers.GetEnv("TEST_SNS_ENDPOINT", "http://localhost:4575")
	Convey("sns: init ", t, func() {
		snsSvc := sns.New(helpers.GetAwsSession(snsLocalStack))

		_, err := snsSvc.CreateTopic(&sns.CreateTopicInput{Name: aws.String("test-" + strconv.Itoa(int(time.Now().Unix())))})
		So(err, ShouldBeNil)
		Convey("sns: std cmd ", func() {
			args := []string{"--sns-endpoint=" + snsLocalStack}
			Command.SetArgs(args)
			err := Command.Execute()
			So(err, ShouldBeNil)
		})
		Convey("sns: max topics 10", func() {
			args := []string{"--sns-endpoint=" + snsLocalStack}
			args = append(args, "--max-topics=10")
			Command.SetArgs(args)
			err := Command.Execute()
			So(err, ShouldBeNil)
		})
		Convey("sns: max topics 0", func() {
			args := []string{"--sns-endpoint=" + snsLocalStack}
			args = append(args, "--max-topics=0")
			Command.SetArgs(args)
			err := Command.Execute()
			So(err, ShouldBeNil)
		})
	})
}
