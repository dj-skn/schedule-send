# Schedule a message: a directory picks the agent, then the job list shows it
# beside the resume job armed out of view. Recorded by tools/media.
Output out/schedule.gif

Set Shell bash
Set FontFamily "JetBrains Mono"
Set FontSize 20
Set Width 1500
Set Height 460
Set Padding 24
Set Theme { "name": "Claude Dark", "background": "#262626", "foreground": "#f8f8f6", "selection": "#1f4569", "cursor": "#f8f8f6", "cursorAccent": "#262626", "black": "#373734", "red": "#ff3a30", "green": "#32d74b", "yellow": "#ffd014", "blue": "#0099ff", "magenta": "#ff2c56", "cyan": "#ffb454", "white": "#c3c2b7", "brightBlack": "#6e6c68", "brightRed": "#ff5047", "brightGreen": "#6fe28a", "brightYellow": "#ffd942", "brightBlue": "#42b4ff", "brightMagenta": "#ff6184", "brightCyan": "#ffb454", "brightWhite": "#fdfdfc" }
Set WindowBar Colorful
Set BorderRadius 10
Set Margin 24
Set MarginFill "#373734"
Set TypingSpeed 45ms

Hide
Type "source /work/env.sh && schedule-send --pane wA:p1 --auto > /dev/null 2>&1; clear"
Enter
Show

Sleep 600ms
Type "schedule-send --cwd ~/projects/website --at 18:30 --message 'run the e2e suite'"
Sleep 500ms
Enter
Sleep 3s
Type "schedule-send --list"
Sleep 400ms
Enter
Sleep 5s
