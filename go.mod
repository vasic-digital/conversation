module digital.vasic.conversation

go 1.24.0

require (
	digital.vasic.messaging v0.0.0-00010101000000-000000000000
	github.com/google/uuid v1.6.0
	github.com/segmentio/kafka-go v0.4.49
	github.com/sirupsen/logrus v1.9.4
	github.com/stretchr/testify v1.10.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// For local development, replaced by root go.mod
replace digital.vasic.messaging => ../Messaging
