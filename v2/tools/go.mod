module github.com/dreamsxin/go-kit-tools/v2

go 1.25.8

require (
	github.com/dreamsxin/go-kit/v2/cmd/microgen v0.1.0
	github.com/mattn/go-sqlite3 v1.14.40
)

require github.com/emicklei/proto v1.14.3 // indirect

replace github.com/dreamsxin/go-kit/v2 => ..

replace github.com/dreamsxin/go-kit/v2/cmd/microgen => ../cmd/microgen
