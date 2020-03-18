package commands

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_RootCommand(t *testing.T) {
	Convey("RootCommand: without args", t, func() {
		err := RootCommand.Execute()
		So(err, ShouldNotBeNil)
	})
}
func Test_Version(t *testing.T) {
	Convey("RootCommand: version", t, func() {
		RootCommand.SetArgs([]string{"version"})
		err := RootCommand.Execute()
		So(err, ShouldBeNil)
	})
}
