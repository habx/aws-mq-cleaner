package sqs

import (
	"testing"

	"github.com/habx/aws-mq-cleaner/helpers"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_SQS(t *testing.T) {
	sqsLocalStack := helpers.GetEnv("TEST_SQS_ENDPOINT", "http://localhost:4576")
	cloudWatchLocalStack := helpers.GetEnv("TEST_CLOUDWATCH_ENDPOINT", "http://localhost:4586")
	Convey("sqs: std cmd ", t, func() {
		args := []string{"--sqs-endpoint=" + sqsLocalStack, "--cloudwatch-endpoint=" + cloudWatchLocalStack}
		Command.SetArgs(args)
		err := Command.Execute()
		So(err, ShouldBeNil)
	})
	Convey("sqs: queue prefix test", t, func() {
		args := []string{"--sqs-endpoint=" + sqsLocalStack, "--cloudwatch-endpoint=" + cloudWatchLocalStack}
		args = append(args, "--queue-prefix=xx")
		Command.SetArgs(args)
		err := Command.Execute()
		So(err, ShouldBeNil)
	})
	Convey("sqs: since 1d", t, func() {
		args := []string{"--sqs-endpoint=" + sqsLocalStack, "--cloudwatch-endpoint=" + cloudWatchLocalStack}
		args = append(args, "--since=1d")
		Command.SetArgs(args)
		err := Command.Execute()
		So(err, ShouldBeNil)
	})
}
