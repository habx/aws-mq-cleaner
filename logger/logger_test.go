package logger_test

import (
	"testing"

	"github.com/habx/aws-mq-cleaner/logger"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_Logger(t *testing.T) {
	Convey("logger: init debug logger", t, func() {
		z := logger.GetLogger("debug")
		So(z, ShouldNotBeNil)
	})

	Convey("logger: init debug logger", t, func() {
		z := logger.GetLogger("info")
		So(z, ShouldNotBeNil)
	})
}
