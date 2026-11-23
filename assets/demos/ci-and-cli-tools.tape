Output "assets/demos/ci-and-cli-tools.gif"

Set Shell "zsh"
Set FontFamily "JetBrainsMono Nerd Font Mono"
Set FontSize 16
Set Width 1220
Set Height 740
Set Padding 22
Set Margin 18
Set MarginFill "#0B1324"
Set BorderRadius 12
Set WindowBar Colorful
Set WindowBarSize 40
Set Theme "Catppuccin Mocha"
Set TypingSpeed 42ms
Set Framerate 30

Hide
Type "setopt INTERACTIVE_COMMENTS"
Enter
Type "unset TERM_PROGRAM"
Enter
Type 'export PS1="%F{#00B14F}❯%f %F{#94A3B8}android-app%f "'
Enter
Type "clear"
Enter
Sleep 400ms
Show

Sleep 400ms
Type "# Full CLI command tree, proxy, Persian RTL, and CI flags"
Enter
Sleep 300ms
Type "bazaar --help"
Enter
Sleep 3.5s

Type "clear"
Enter
Sleep 300ms

Type "# Dynamic shell autocomplete powered by your Gradle build types"
Enter
Sleep 300ms
Type 'bazaar __complete release status --app ""'
Enter
Sleep 2.5s

Type "# Headless CI/CD JSON output (raw UTF-8 preserved)"
Enter
Sleep 300ms
Type "bazaar release status --only published --output json"
Enter
Sleep 4s
