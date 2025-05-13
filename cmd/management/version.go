package main

import "time"

var (
	Version   = "dev"
	BuildDate = time.Now().Format(time.RFC3339)
)
