Output "assets/demos/android-gradle.gif"

Set Shell "zsh"
Set FontFamily "JetBrainsMono Nerd Font Mono"
Set FontSize 16
Set Width 1220
Set Height 760
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
Type "# Auto-detect Android Gradle modules, variants, and Git state"
Enter
Sleep 300ms
Type "bazaar android info"
Enter
Sleep 3.5s

Type "clear"
Enter
Sleep 300ms

Type "# List every Gradle build type and flavor with resolved package names"
Enter
Sleep 300ms
Type "bazaar android variants"
Enter
Sleep 2.5s

Type "# Preview a safe versionCode + versionName bump (no files edited)"
Enter
Sleep 300ms
Type "bazaar android bump --bump patch --dry-run"
Enter
Sleep 3.5s
