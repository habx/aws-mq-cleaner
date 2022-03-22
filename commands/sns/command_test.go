package sns_test

import (
	"strconv"
	"time"

	"github.com/habx/aws-mq-cleaner/commands"

	"github.com/aws/aws-sdk-go/aws"

	"testing"

	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/habx/aws-mq-cleaner/helpers"
	. "github.com/smartystreets/goconvey/convey"
)

var snsLocalStack = helpers.GetEnv("TEST_SNS_ENDPOINT", "http://localhost.localstack.cloud:4566")

func Test_SNS(t *testing.T) {

	Convey("sns: bad args ", t, func() {
		Convey("sns: no args ", func() {
			err := commands.RootCommand.Execute()
			So(err, ShouldBeNil)
		})
	})

	Convey("sns: std ", t, func() {
		CreateTopic()
		Convey("sns: std cmd ", func() {
			args := []string{"--loglevel=debug", "sns", "--sns-endpoint=" + snsLocalStack}
			commands.RootCommand.SetArgs(args)
			err := commands.RootCommand.Execute()
			So(err, ShouldBeNil)
		})

		Convey("sns: max topics 0", func() {
			args := []string{"--loglevel=debug", "sns", "--sns-endpoint=" + snsLocalStack, "--max-topics=0"}
			commands.RootCommand.SetArgs(args)
			err := commands.RootCommand.Execute()
			So(err, ShouldBeNil)
		})
		Convey("sns: max topics 10", func() {
			args := []string{"--loglevel=debug", "sns", "--sns-endpoint=" + snsLocalStack, "--max-topics=10"}
			commands.RootCommand.SetArgs(args)
			err := commands.RootCommand.Execute()
			So(err, ShouldBeNil)
		})

		CreateTopicWithDateNow()
		CreateTopicWithDate()
		Convey("sns: custom tag for update date", func() {
			args := []string{"--loglevel=debug", "sns", "--sns-endpoint=" + snsLocalStack, "--check-tag-name-update-date=update_date", "-d=true", "--since=1s"}
			commands.RootCommand.SetArgs(args)
			err := commands.RootCommand.Execute()
			So(err, ShouldBeNil)
		})
		CreateTopicWithDateNow()
		CreateTopicWithDate()
		Convey("sns: exclude pattern with test value", func() {
			args := []string{"--loglevel=debug", "--exclude-patten=test-", "sns", "--sns-endpoint=" + snsLocalStack}
			commands.RootCommand.SetArgs(args)
			err := commands.RootCommand.Execute()
			So(err, ShouldBeNil)
		})

		Convey("sns: topic prefix with test value", func() {
			args := []string{"--loglevel=debug", "sns", "--sns-endpoint=" + snsLocalStack, "--topic-prefix=test"}
			commands.RootCommand.SetArgs(args)
			commands.RootCommand.Execute()
			//So(err, ShouldBeNil)
		})

		Convey("sns: enable delete and header", func() {
			args := []string{"--loglevel=debug", "sns", "--sns-endpoint=" + snsLocalStack, "-d=true", "--no-header=true", "--topic-prefix="}
			commands.RootCommand.SetArgs(args)
			err := commands.RootCommand.Execute()
			So(err, ShouldBeNil)
		})
	})
}

func CreateTopic() {
	snsSvc := sns.New(helpers.GetAwsSession(snsLocalStack))
	arn, err := snsSvc.CreateTopic(&sns.CreateTopicInput{Name: aws.String("test-" + strconv.Itoa(int(time.Now().Unix())))})
	if err != nil {
		panic(err)
	}
	_, err = snsSvc.TagResource(&sns.TagResourceInput{
		ResourceArn: arn.TopicArn,
		Tags: []*sns.Tag{
			{
				Key:   aws.String("update_date"),
				Value: aws.String("2021-01-24T00:00:00.000-03:00"),
			},
		},
	})
	if err != nil {
		panic(err)
	}
}

func CreateTopicWithDateNow() {
	snsSvc := sns.New(helpers.GetAwsSession(snsLocalStack))
	arn, err := snsSvc.CreateTopic(&sns.CreateTopicInput{Name: aws.String("test-" + strconv.Itoa(int(time.Now().Unix())))})
	if err != nil {
		panic(err)
	}
	_, err = snsSvc.TagResource(&sns.TagResourceInput{
		ResourceArn: arn.TopicArn,
		Tags: []*sns.Tag{
			{
				Key:   aws.String("update_date"),
				Value: aws.String(time.Now().Format("2006-01-02T15:04:05.000-03:00")),
			},
		},
	})
	if err != nil {
		panic(err)
	}
}

func CreateTopicWithDate() {
	snsSvc := sns.New(helpers.GetAwsSession(snsLocalStack))
	arn, err := snsSvc.CreateTopic(&sns.CreateTopicInput{Name: aws.String("test-" + strconv.Itoa(int(time.Now().Unix())))})
	if err != nil {
		panic(err)
	}
	_, err = snsSvc.TagResource(&sns.TagResourceInput{
		ResourceArn: arn.TopicArn,
		Tags: []*sns.Tag{
			{
				Key:   aws.String("update_date"),
				Value: aws.String("2021-01-24T00:00:00.000-03:00"),
			},
		},
	})
	if err != nil {
		panic(err)
	}
}
