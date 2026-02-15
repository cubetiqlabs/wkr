package main

import (
	"fmt"
	"os"
)

func cmdLogout() {
	if err := os.Remove(credentialsPath()); err != nil && !os.IsNotExist(err) {
		fatal("failed to remove credentials: " + err.Error())
	}
	fmt.Println("✓ Logged out")
}
