# Schedule a message: a directory picks the agent, then the job list shows it
# beside the resume job armed out of view. Recorded by tools/media.
Output out/schedule.gif

Set Shell bash
Set FontFamily "JetBrains Mono"
Set FontSize 20
Set Width 1500
Set Height 460
Set Padding 24
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set BorderRadius 10
Set Margin 24
Set MarginFill "#45475a"
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
