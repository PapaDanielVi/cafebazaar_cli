Output "assets/demos/publish-from-scratch.gif"

Set Shell "zsh"
Set FontFamily "JetBrainsMono Nerd Font Mono"
Set FontSize 16
Set Width 1240
Set Height 760
Set Padding 22
Set Margin 18
Set MarginFill "#0B1324"
Set BorderRadius 12
Set WindowBar Colorful
Set WindowBarSize 40
Set Theme "Catppuccin Mocha"
Set TypingSpeed 20ms
Set Framerate 30

Hide
Type "setopt INTERACTIVE_COMMENTS"
Enter
Type "unset TERM_PROGRAM"
Enter
Type "export BAZAAR_RTL=on"
Enter
Type 'export PS1="%F{#00B14F}❯%f %F{#94A3B8}simple-notes%f "'
Enter
Type "clear"
Enter
Sleep 400ms
Show

Sleep 400ms
Type "# 1. Initialize a brand-new Android app and submit v1.0.0 to CafeBazaar"
Enter
Sleep 300ms
Type 'bazaar init --mode new --variant release --artifact app/build/outputs/apk/release/app-release.apk \'
Enter
Type '  --name-fa "Simple Notes" --name-en "Simple Notes" --email dev@example.com --phone "+982100000000" --website "https://example.com" \'
Enter
Type '  --short-description-fa "Fast notes app" --short-description-en "Fast notes app" \'
Enter
Type '  --description-fa-file assets/desc-fa.txt --description-en-file assets/desc-en.txt \'
Enter
Type '  --icon assets/icon.png --screenshot assets/screen1.png'
Enter
Sleep 1.2s

# Filter the interactive category picker for Tools [11] and select it
Type "/"
Sleep 250ms
Type "tools"
Sleep 1s
Enter
Sleep 1.2s

# Confirm the DANGER card to create and submit the brand-new app
Left
Sleep 600ms
Enter
Sleep 3.5s

Type "# Generated .bazaar.yaml includes JSON Schema support and never stores secrets"
Enter
Sleep 300ms
Type "cat .bazaar.yaml"
Enter
Sleep 2.5s

Type "clear"
Enter
Sleep 350ms

Type "# 2. Ship v1.0.1: bump Gradle, build APK, commit/tag Git, upload & publish"
Enter
Sleep 300ms
Type 'bazaar publish --bump patch --build --git-commit --git-tag --changelog-fa-file notes/fa.txt --changelog-en "Speed improvements and bug fixes"'
Enter
Sleep 1.5s

# Staged rollout prompt defaults to Yes (100%); select No to set a custom 50% rollout
Right
Sleep 500ms
Enter
Sleep 800ms
Backspace
Backspace
Sleep 200ms
Type "50"
Sleep 450ms
Enter
Sleep 1s

# Auto-publish prompt defaults to No; toggle Yes so CafeBazaar publishes automatically after review
Left
Sleep 550ms
Enter
Sleep 4.2s

# Confirm the DANGER card to commit and submit v1.0.1 for review
Left
Sleep 650ms
Enter
Sleep 4.5s
