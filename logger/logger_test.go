package logger

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_Logger(t *testing.T) {
	Convey("logger: init debug logger", t, func() {
		z := GetLogger("debug")
		So(z, ShouldNotBeNil)
	})

	Convey("logger: init debug logger", t, func() {
		z := GetLogger("info")
		So(z, ShouldNotBeNil)
	})
}
