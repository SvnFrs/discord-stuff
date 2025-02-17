package main

import (
	"fmt"
	"time"

	"example.com/presence/lib/client"
)

func main() {
	err := client.Login("")
	if err != nil {
		panic(err)
	}
	fmt.Println("Logged in")

	now := time.Now()
	err = client.SetActivity(client.Activity{
		State:      "Co gi hot?",
		Details:    "OS: Arch Linux x86_64\n Kernel: 6.13.1-zen1-1-zen \n Packages: 938 (pacman) \n Shell: zsh 5.9",
		LargeImage: "largeimageid",
		LargeText:  "This is the large image :D",
		SmallImage: "smallimageid",
		SmallText:  "And this is the small image",
		// Party: &client.Party{
		// 	ID:         "-1",
		// 	Players:    1,
		// 	MaxPlayers: 1,
		// },
		Timestamps: &client.Timestamps{
			Start: &now,
		},
		Buttons: []*client.Button{
			{
				Label: "GitHub",
				Url:   "https://github.com/hugolgst/rich-go",
			},
		},
	})

	if err != nil {
		panic(err)
	}

	// discord will only show the presence if the app is running
	go forever()
	select {}

}

func forever() {
	for {
		fmt.Printf("%v+\n...", time.Now())
		time.Sleep(10 * time.Second)
	}
}
