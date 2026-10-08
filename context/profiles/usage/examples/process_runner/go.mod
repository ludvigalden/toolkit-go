module example.com/process_runner

go 1.26.0

require (
	github.com/alecthomas/kong v1.16.1
	github.com/ludvigalden/toolkit-go v0.0.0
)

require golang.org/x/sys v0.49.0 // indirect

replace github.com/ludvigalden/toolkit-go => ../../../../..
