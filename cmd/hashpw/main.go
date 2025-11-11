package main

import (
	"crypto/sha256"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: hashpw <password>")
		fmt.Println("Example: hashpw mypassword")
		os.Exit(1)
	}

	password := os.Args[1]
	hash := sha256.Sum256([]byte(password))
	hashStr := fmt.Sprintf("%x", hash)

	fmt.Printf("Password: %s\n", password)
	fmt.Printf("SHA-256 Hash: %s\n", hashStr)
	fmt.Println("\nAdd this to your config:")
	fmt.Printf("  web_auth:\n")
	fmt.Printf("    enabled: true\n")
	fmt.Printf("    username: admin\n")
	fmt.Printf("    password_hash: \"%s\"\n", hashStr)
}
