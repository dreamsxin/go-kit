module example.com/gen_idl_extend_append

go 1.25.8

require (
	github.com/dreamsxin/go-kit v0.0.0
	github.com/gorilla/mux v1.8.1
	github.com/sony/gobreaker v1.0.0
	golang.org/x/time v0.15.0
	gorm.io/gorm v1.31.1
)

require (
	github.com/streadway/handy v0.0.0-20200128134331-0f66f006fb2e // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
)

replace github.com/dreamsxin/go-kit => ../../../
