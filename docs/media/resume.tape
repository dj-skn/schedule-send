# Resume after a usage limit: the picker puts the limited agent first, Enter
# takes it, Enter keeps "resume", and Enter again schedules it a minute after
# the reset.
# Recorded by tools/media.
Output out/resume.gif

Set Shell bash
Set FontFamily "JetBrains Mono"
Set FontSize 20
Set Width 1560
Set Height 960
Set Padding 24
Set Theme { "name": "Claude Dark", "background": "#262626", "foreground": "#f8f8f6", "selection": "#1f4569", "cursor": "#f8f8f6", "cursorAccent": "#262626", "black": "#373734", "red": "#ff3a30", "green": "#32d74b", "yellow": "#ffd014", "blue": "#0099ff", "magenta": "#ff2c56", "cyan": "#ffb454", "white": "#c3c2b7", "brightBlack": "#6e6c68", "brightRed": "#ff5047", "brightGreen": "#6fe28a", "brightYellow": "#ffd942", "brightBlue": "#42b4ff", "brightMagenta": "#ff6184", "brightCyan": "#ffb454", "brightWhite": "#fdfdfc" }
Set WindowBar Colorful
Set BorderRadius 10
Set Margin 24
Set MarginFill "#373734"
Set TypingSpeed 55ms

Hide
Type "source /work/env.sh && clear"
Enter
Show

Sleep 600ms
Type "schedule-send"
Sleep 500ms
Enter
Sleep 2.4s
Enter
Sleep 2s
Enter
Sleep 2.6s
Enter
Sleep 5s
