// Random worktree branch codenames, Conductor/Docker-style: "adjective-city"
// (e.g. "quiet-austin"). Generated once at worktree creation time and never
// renamed — decoupled from the session title so branch/worktree naming
// never races auto-titling and never needs a risky git-branch rename.
package services

import (
	"math/rand"
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
// "bright-denver". The same value doubles as the worktree directory name
// (<root>/<codename>), so callers retry on collision — the word-pair space
// is small and concurrent sessions can roll the same pair.
func RandomCodename() string {
	adj := codenameAdjectives[rand.Intn(len(codenameAdjectives))]
	city := codenameCities[rand.Intn(len(codenameCities))]
	return adj + "-" + city
}
