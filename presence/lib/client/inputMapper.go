package client

import (
	"time"
)

// Activity holds the data for discord rich presence
type Activity struct {
	Details    string
	State      string
	LargeImage string
	LargeText  string
	SmallImage string
	SmallText  string
	Party      *Party
	Timestamps *Timestamps
	Secrets    *Secrets
	Buttons    []*Button
}

// Button holds a label and corresponding URL.
type Button struct {
	Label string
	Url   string
}

// Party holds information about the party.
type Party struct {
	ID         string
	Players    int
	MaxPlayers int
}

// Timestamps holds start (and end) times.
type Timestamps struct {
	Start *time.Time
	End   *time.Time
}

// Secrets holds secrets for joining/spectating.
type Secrets struct {
	Match    string
	Join     string
	Spectate string
}

func mapActivity(activity *Activity) *PayloadActivity {
	final := &PayloadActivity{
		Details: activity.Details,
		State:   activity.State,
		Assets: PayloadAssets{
			LargeImage: activity.LargeImage,
			LargeText:  activity.LargeText,
			SmallImage: activity.SmallImage,
			SmallText:  activity.SmallText,
		},
	}

	if activity.Timestamps != nil && activity.Timestamps.Start != nil {
		start := uint64(activity.Timestamps.Start.UnixNano() / 1e6)
		final.Timestamps = &PayloadTimestamps{
			Start: &start,
		}
		if activity.Timestamps.End != nil {
			end := uint64(activity.Timestamps.End.UnixNano() / 1e6)
			final.Timestamps.End = &end
		}
	}

	if activity.Party != nil {
		final.Party = &PayloadParty{
			ID:   activity.Party.ID,
			Size: [2]int{activity.Party.Players, activity.Party.MaxPlayers},
		}
	}

	if activity.Secrets != nil {
		final.Secrets = &PayloadSecrets{
			Join:     activity.Secrets.Join,
			Match:    activity.Secrets.Match,
			Spectate: activity.Secrets.Spectate,
		}
	}

	if len(activity.Buttons) > 0 {
		// pre-allocate the button slice to avoid repeated memory grows
		final.Buttons = make([]*PayloadButton, 0, len(activity.Buttons))
		for _, btn := range activity.Buttons {
			final.Buttons = append(final.Buttons, &PayloadButton{
				Label: btn.Label,
				Url:   btn.Url,
			})
		}
	}

	return final
}
