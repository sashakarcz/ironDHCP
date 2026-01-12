package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: hashpw <password>")
		fmt.Println("Example: hashpw mypassword")
		os.Exit(1)
	}

	password := os.Args[1]
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Printf("Error hashing password: %v\n", err)
		os.Exit(1)
	}
	hashStr := string(bytes)

	fmt.Printf("Password: %s\n", password)
	fmt.Printf("Bcrypt Hash: %s\n", hashStr)
	fmt.Println("\nAdd this to your config:")
	fmt.Printf("  web_auth:\n")
	fmt.Printf("    enabled: true\n")
	fmt.Printf("    username: admin\n")
	fmt.Printf("    password_hash: '%s'\n", hashStr)
}
