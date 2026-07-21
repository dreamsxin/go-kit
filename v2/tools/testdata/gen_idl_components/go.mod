module example.com/gen_idl_components

go 1.25.8

require (
	github.com/dreamsxin/go-kit/v2 v2.0.0
	github.com/sony/gobreaker v1.0.0
	github.com/swaggest/swgui v1.8.9
	golang.org/x/time v0.15.0
	gorm.io/gorm v1.31.1
)

require (
	github.com/shurcooL/httpgzip v0.0.0-20190720172056-320755c1c1b0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/text v0.34.0 // indirect
)

replace github.com/dreamsxin/go-kit/v2 => ../../..
