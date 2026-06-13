# VHS tape for the hero GIF — https://github.com/charmbracelet/vhs
# Build the binary first, then:  vhs docs/demo.tape   ->   docs/demo.gif
#
# It runs the demo with a low $5 budget so the bar crosses into red on camera.

Output docs/demo.gif

Set Theme "Catppuccin Mocha"
Set FontSize 16
Set Width 1200
Set Height 700
Set Padding 18
Set PlaybackSpeed 1.0

Hide
Type "go build -o /tmp/toktop . && export TOKTOP_BUDGET=5"
Enter
Sleep 2s
Show

Type "toktop --demo"
Enter

# Let the dollars tick and the budget bar fill from green -> amber -> red.
Sleep 22s

# Toggle sort to spend, linger on the breakdown.
Type "s"
Sleep 4s

Type "q"
Sleep 1s
