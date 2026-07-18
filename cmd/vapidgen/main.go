// Command vapidgen prints a fresh VAPID key pair for Web Push.
//
//	go run ./cmd/vapidgen
//
// Set the output as MC_VAPID_PUBLIC_KEY / MC_VAPID_PRIVATE_KEY (and
// MC_VAPID_SUBJECT) on the server to enable blocked-session push notifications.
package main

import (
	"fmt"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		panic(err)
	}
	fmt.Println("# Web Push VAPID keys — set these on the server:")
	fmt.Printf("MC_VAPID_PUBLIC_KEY=%s\n", pub)
	fmt.Printf("MC_VAPID_PRIVATE_KEY=%s\n", priv)
	fmt.Println("MC_VAPID_SUBJECT=mailto:you@example.com")
}
