package main

import _ "embed"

//go:embed commands.json
var redisCommandsJSON []byte
