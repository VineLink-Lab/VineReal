module github.com/VineLink-Lab/VineReal/test/e2e

go 1.27.0

replace github.com/VineLink-Lab/VineReal/shared => ../../shared

replace github.com/VineLink-Lab/VineReal/client => ../../client

replace github.com/VineLink-Lab/VineReal/server => ../../server

require (
	github.com/VineLink-Lab/VineReal/client v0.0.0
	github.com/VineLink-Lab/VineReal/server v0.0.0
	github.com/VineLink-Lab/VineReal/shared v0.0.0
)
