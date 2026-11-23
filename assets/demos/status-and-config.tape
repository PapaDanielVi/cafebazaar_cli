Output "assets/demos/status-and-config.gif"

Set Shell "zsh"
Set FontFamily "JetBrainsMono Nerd Font Mono"
Set FontSize 16
Set Width 1240
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
Type "export BAZAAR_RTL=on"
Enter
Type 'export PS1="%F{#00B14F}❯%f %F{#94A3B8}android-app%f "'
Enter
Type "clear"
Enter
Sleep 400ms
Show

Sleep 400ms
Type "# Non-secret project configuration (.bazaar.yaml)"
Enter
Sleep 300ms
Type "cat .bazaar.yaml"
Enter
Sleep 2.5s

Type "# Check Nasher developer token, saved app keys, and folder binding"
Enter
Sleep 300ms
Type "bazaar auth status"
Enter
Sleep 3s

Type "clear"
Enter
Sleep 300ms

Type "# Live CafeBazaar release status, Gradle version, Git state, and Farsi notes"
Enter
Sleep 300ms
Type "bazaar status"
Enter
Sleep 5s
