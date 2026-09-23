// Random worktree branch codenames, Conductor/Docker-style: "adjective-city"
// (e.g. "quiet-austin"). Generated once at worktree creation time and never
// renamed — decoupled from the session title so branch/worktree naming
// never races auto-titling and never needs a risky git-branch rename.
package services

import (
	"math/rand"
	"strings"
)

var codenameAdjectives = []string{
	"quiet", "bright", "swift", "calm", "bold", "lucky", "sharp", "cozy",
	"wild", "merry", "brave", "clever", "gentle", "jolly", "keen", "misty",
	"noble", "plain", "rapid", "silent", "tidy", "vivid", "warm", "young",
	"zesty",
}

var codenameCities = []string{
	"austin", "denver", "boston", "dallas", "phoenix", "tucson", "miami",
	"atlanta", "chicago", "seattle", "portland", "fresno", "tampa", "dayton",
	"buffalo", "newark", "salem", "aspen", "sedona", "tahoe", "olympia",
	"juneau", "helena", "laramie", "marfa", "savannah", "charleston",
	"raleigh", "tulsa", "wichita",
}

// RandomCodename returns a random "adjective-city" branch name, e.g.
// "bright-denver". id is used to derive a short, deterministic-per-call
// tiebreaker suffix so concurrent sessions can never collide on the same
// branch name even if they happen to roll the same word pair.
func RandomCodename(id string) string {
	adj := codenameAdjectives[rand.Intn(len(codenameAdjectives))]
	city := codenameCities[rand.Intn(len(codenameCities))]
	base := adj + "-" + city

	short := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, strings.ToLower(id))
	if len(short) > 6 {
		short = short[:6]
	}
	if short == "" {
		return base
	}
	return base + "-" + short
}
