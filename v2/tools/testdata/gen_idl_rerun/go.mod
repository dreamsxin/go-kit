module example.com/gen_idl_rerun

go 1.25.8

require (
	github.com/dreamsxin/go-kit/v2 v2.0.0
	github.com/sony/gobreaker v1.0.0
	github.com/swaggest/swgui v1.8.9
	golang.org/x/time v0.11.0
	gorm.io/gorm v1.31.1
	github.com/spf13/viper v1.20.1
	github.com/spf13/viper/remote v1.21.0
	go.uber.org/zap v1.27.0
)


replace github.com/dreamsxin/go-kit/v2 => ../../..

require example.com/custom v0.0.0
