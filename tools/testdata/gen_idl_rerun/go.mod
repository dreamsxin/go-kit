module example.com/gen_idl_rerun

go 1.25.8

require (
	github.com/dreamsxin/go-kit v0.0.0
	github.com/gorilla/mux v1.8.1
	github.com/sony/gobreaker v1.0.0
	golang.org/x/time v0.11.0
	gorm.io/gorm v1.31.1
)


replace github.com/dreamsxin/go-kit => ../../../


require example.com/custom v0.0.0
