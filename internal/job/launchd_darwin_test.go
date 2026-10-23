package job

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const agentID = "20261003-160100-wBp1"

// standin puts the launchctl stand-in first on PATH and returns the paths of
// a job under a temporary root, with where its calls are logged.
func standin(t *testing.T) (Paths, string) {
	t.Helper()
	bin, err := filepath.Abs("../../testdata/standin/launchctl")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "path")
	loaded := filepath.Join(dir, "loaded")
	for _, d := range []string{path, loaded} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(bin, filepath.Join(path, "launchctl")); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "launchctl.log")
	t.Setenv("PATH", path+":"+os.Getenv("PATH"))
	t.Setenv("LAUNCHCTL_STANDIN_LOG", log)
	t.Setenv("LAUNCHCTL_STANDIN_DIR", loaded)
	t.Setenv("LAUNCHCTL_STANDIN_FAIL", "")
	return Paths{StateDir: filepath.Join(dir, "state"), UnitDir: filepath.Join(dir, "agents")}, log
}

func calls(t *testing.T, log string) []string {
	t.Helper()
	b, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var verbs []string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		verbs = append(verbs, strings.Fields(line)[0])
	}
	return verbs
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// armed publishes a job's state and loads its agent, as scheduling does.
func armed(t *testing.T, p Paths, runAt int64, persistent bool) {
	t.Helper()
	msg := "resume"
	if err := p.Write(State{ID: agentID, Kind: "agent", PaneID: "wB:p1", Agent: "claude", Cwd: "/home/dev", Mode: "text", RunAt: runAt}, &msg); err != nil {
		t.Fatal(err)
	}
	trigger, err := Scheduler{}.Render("wB:p1", "2026-10-03 16:01:00", "/opt/schedule-send", agentID, runAt, persistent)
	if err != nil {
		t.Fatal(err)
	}
	if attempted, err := (Scheduler{}).Arm(p, trigger); err != nil || !attempted {
		t.Fatalf("arm: attempted=%v err=%v", attempted, err)
	}
}

func TestAgentCarriesTheCalendarInTheSystemZone(t *testing.T) {
	prev := zone
	zone = func() *time.Location { return time.FixedZone("test", 2*3600) }
	t.Cleanup(func() { zone = prev })
	t.Setenv("PATH", "/opt/homebrew/bin:/usr/bin")
	t.Setenv("XDG_STATE_HOME", "/tmp/a&b")
	t.Setenv("XDG_CONFIG_HOME", "")
	p := Paths{StateDir: "/state", UnitDir: "/agents"}
	// 2026-10-03 14:01:00 UTC is 16:01 in the test zone.
	got := string(agent(p, Trigger{id: agentID, self: `/opt/a "b" <c>/schedule-send`, runAt: 1791036060}, false))
	for _, want := range []string{
		"<key>Label</key>\n\t<string>herdr-send-" + agentID + "</string>",
		"<string>/opt/a &#34;b&#34; &lt;c&gt;/schedule-send</string>\n\t\t<string>__fire</string>\n\t\t<string>" + agentID + "</string>",
		"<key>Month</key>\n\t\t<integer>10</integer>\n\t\t<key>Day</key>\n\t\t<integer>3</integer>\n\t\t<key>Hour</key>\n\t\t<integer>16</integer>\n\t\t<key>Minute</key>\n\t\t<integer>1</integer>",
		"<key>StandardOutPath</key>\n\t<string>/state/" + agentID + ".log</string>",
		"<key>PATH</key>\n\t\t<string>/opt/homebrew/bin:/usr/bin</string>",
		"<key>XDG_STATE_HOME</key>\n\t\t<string>/tmp/a&amp;b</string>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("agent lacks %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"RunAtLoad", "XDG_CONFIG_HOME"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("agent has %q:\n%s", unwanted, got)
		}
	}
}

func TestAgentTriggers(t *testing.T) {
	p := Paths{StateDir: "/state", UnitDir: "/agents"}
	cases := []struct {
		name               string
		atLoad, persistent bool
		calendar, load     bool
	}{
		{"calendar only", false, false, true, false},
		{"persistent keeps the calendar and runs at login", false, true, true, true},
		{"due soon runs when loaded", true, false, false, true},
		{"due soon and persistent", true, true, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(agent(p, Trigger{id: agentID, self: "/opt/schedule-send", runAt: 1791036060, persistent: c.persistent}, c.atLoad))
			if strings.Contains(got, "StartCalendarInterval") != c.calendar || strings.Contains(got, "RunAtLoad") != c.load {
				t.Errorf("calendar=%v load=%v wanted:\n%s", c.calendar, c.load, got)
			}
		})
	}
}

func TestOnlyAPersistentJobIsWrittenWhereLaunchdLoadsAtLogin(t *testing.T) {
	p, _ := standin(t)
	armed(t, p, time.Now().Add(time.Hour).Unix(), false)
	beside, atLogin := p.agentPath(agentID, false), p.agentPath(agentID, true)
	if filepath.Dir(beside) != p.StateDir || filepath.Dir(atLogin) != p.UnitDir {
		t.Fatalf("agent paths %q and %q", beside, atLogin)
	}
	if !exists(beside) || exists(atLogin) || exists(p.UnitDir) {
		t.Fatalf("ordinary job: beside its state=%v, login folder=%v", exists(beside), exists(atLogin))
	}
	if got := (Scheduler{}).Files(p, agentID, false); got != beside {
		t.Errorf("receipt names %q, want %q", got, beside)
	}
	if err := p.Remove(Scheduler{}, agentID); err != nil {
		t.Fatal(err)
	}

	armed(t, p, time.Now().Add(time.Hour).Unix(), true)
	if exists(beside) || !exists(atLogin) {
		t.Fatalf("persistent job: beside its state=%v, login folder=%v", exists(beside), exists(atLogin))
	}
	if b, _ := os.ReadFile(atLogin); !strings.Contains(string(b), "RunAtLoad") {
		t.Errorf("persistent agent does not run at login:\n%s", b)
	}
	if err := p.Remove(Scheduler{}, agentID); err != nil || exists(atLogin) {
		t.Fatalf("remove: %v, agent left=%v", err, exists(atLogin))
	}
}

func TestRenderRefusesWhatAnAgentCannotCarry(t *testing.T) {
	for name, self := range map[string]string{
		"relative":  "bin/schedule-send",
		"control":   "/opt/a\x01b",
		"not UTF-8": "/opt/\xff",
	} {
		if _, err := (Scheduler{}).Render("wB:p1", "", self, agentID, 1, false); err == nil {
			t.Errorf("%s executable accepted", name)
		}
	}
	if _, err := (Scheduler{}).Render("wB:p1", "", "/opt/schedule-send", "../x", 1, false); err == nil {
		t.Error("invalid id accepted")
	}
}

func TestArmLoadsAndRemoveUnloads(t *testing.T) {
	p, log := standin(t)
	armed(t, p, time.Now().Add(time.Hour).Unix(), false)
	b, err := os.ReadFile(p.agentPath(agentID, false))
	if err != nil || !strings.Contains(string(b), "StartCalendarInterval") {
		t.Fatalf("agent: %v\n%s", err, b)
	}
	if got := (Scheduler{}).Armed(agentID); got != "active" {
		t.Errorf("armed = %q, want active", got)
	}
	if err := p.Remove(Scheduler{}, agentID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{p.agentPath(agentID, false), p.StatePath(agentID), p.MessagePath(agentID)} {
		if exists(path) {
			t.Errorf("%s left behind", path)
		}
	}
	if got := (Scheduler{}).Armed(agentID); got != "" {
		t.Errorf("armed = %q after removal", got)
	}
	if got := strings.Join(calls(t, log), " "); got != "bootstrap print bootout print" {
		t.Errorf("launchctl calls %q", got)
	}
}

func TestAJobDueSoonRunsWhenLoaded(t *testing.T) {
	p, _ := standin(t)
	armed(t, p, time.Now().Add(30*time.Second).Unix(), false)
	b, _ := os.ReadFile(p.agentPath(agentID, false))
	if got := string(b); strings.Contains(got, "StartCalendarInterval") || !strings.Contains(got, "RunAtLoad") {
		t.Fatalf("agent due in 30 s:\n%s", got)
	}
}

func TestFailedLoadReportsAnAttempt(t *testing.T) {
	p, _ := standin(t)
	t.Setenv("LAUNCHCTL_STANDIN_FAIL", "bootstrap")
	trigger, _ := Scheduler{}.Render("wB:p1", "", "/opt/schedule-send", agentID, time.Now().Add(time.Hour).Unix(), false)
	attempted, err := Scheduler{}.Arm(p, trigger)
	if err == nil || !attempted || !strings.Contains(err.Error(), "Could not arm herdr-send-"+agentID) {
		t.Fatalf("attempted=%v err=%v", attempted, err)
	}
	t.Setenv("LAUNCHCTL_STANDIN_FAIL", "")
	if err := p.Rollback(Scheduler{}, agentID, attempted); err != nil || exists(p.agentPath(agentID, false)) {
		t.Fatalf("rollback: %v, agent left=%v", err, exists(p.agentPath(agentID, false)))
	}
}

func TestCancelKeepsStateWhenUnloadingFails(t *testing.T) {
	p, _ := standin(t)
	armed(t, p, time.Now().Add(time.Hour).Unix(), false)
	t.Setenv("LAUNCHCTL_STANDIN_FAIL", "bootout")
	if err := p.Remove(Scheduler{}, agentID); err == nil || !strings.Contains(err.Error(), "Input/output error") {
		t.Fatalf("err = %v", err)
	}
	for _, path := range []string{p.agentPath(agentID, false), p.StatePath(agentID), p.MessagePath(agentID)} {
		if !exists(path) {
			t.Errorf("%s removed although the job is still loaded", path)
		}
	}
}

func TestCancelToleratesAJobThatIsNotLoaded(t *testing.T) {
	p, _ := standin(t)
	armed(t, p, time.Now().Add(time.Hour).Unix(), false)
	if err := (Scheduler{}).bootout(agentID); err != nil {
		t.Fatal(err)
	}
	if err := p.Remove(Scheduler{}, agentID); err != nil || exists(p.StatePath(agentID)) {
		t.Fatalf("remove: %v, state left=%v", err, exists(p.StatePath(agentID)))
	}
}

func TestCleanupRemovesEverythingBeforeBootingItselfOut(t *testing.T) {
	p, log := standin(t)
	armed(t, p, time.Now().Add(time.Hour).Unix(), false)
	// launchd kills a job on bootout, so its state must already be gone.
	t.Setenv("LAUNCHCTL_STANDIN_FAIL", "bootout")
	if err := p.Cleanup(Scheduler{}, agentID); err == nil {
		t.Fatal("a failed bootout was not reported")
	}
	for _, path := range []string{p.agentPath(agentID, false), p.StatePath(agentID), p.MessagePath(agentID)} {
		if exists(path) {
			t.Errorf("%s still there when bootout ran", path)
		}
	}
	if got := calls(t, log); got[len(got)-1] != "bootout" {
		t.Errorf("last launchctl call %q, want bootout", got[len(got)-1])
	}
}

func TestSweepUnloadsOrphansAndOldLogs(t *testing.T) {
	p, _ := standin(t)
	armed(t, p, time.Now().Add(time.Hour).Unix(), false)
	if err := os.Remove(p.StatePath(agentID)); err != nil {
		t.Fatal(err)
	}
	old, fresh := p.LogPath("old"), p.LogPath("fresh")
	for _, l := range []string{old, fresh, p.LogPath(agentID)} {
		if err := os.WriteFile(l, []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	p.Sweep(Scheduler{})
	if exists(p.agentPath(agentID, false)) || (Scheduler{}).Armed(agentID) != "" {
		t.Error("orphan agent left loaded")
	}

	armed(t, p, time.Now().Add(time.Hour).Unix(), true)
	if err := os.Remove(p.StatePath(agentID)); err != nil {
		t.Fatal(err)
	}
	p.Sweep(Scheduler{})
	if exists(p.agentPath(agentID, true)) || (Scheduler{}).Armed(agentID) != "" {
		t.Error("orphan persistent agent left loaded")
	}
	if exists(old) || !exists(fresh) {
		t.Errorf("old log left=%v, fresh log kept=%v", exists(old), exists(fresh))
	}
}

func TestInspectShowsTheJobAndTheEndOfItsLog(t *testing.T) {
	p, _ := standin(t)
	armed(t, p, time.Now().Add(time.Hour).Unix(), false)
	var log strings.Builder
	for i := 0; i < 45; i++ {
		log.WriteString("line\n")
	}
	log.WriteString("last")
	if err := os.WriteFile(p.LogPath(agentID), []byte(log.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	in := Scheduler{}.Inspect(p, agentID, time.Second, time.Second, time.Second)
	if !strings.Contains(in.Timer, "state = not running") || len(in.Diagnostics) != 0 {
		t.Errorf("timer %q diagnostics %+v", in.Timer, in.Diagnostics)
	}
	if lines := strings.Split(in.Journal, "\n"); len(lines) != 40 || lines[39] != "last" {
		t.Errorf("journal has %d lines ending %q", len(lines), lines[len(lines)-1])
	}

	if err := (Scheduler{}).bootout(agentID); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p.LogPath(agentID)); err != nil {
		t.Fatal(err)
	}
	in = Scheduler{}.Inspect(p, agentID, time.Second, time.Second, time.Second)
	if !strings.Contains(in.Timer, "is not loaded") || in.Journal != "" || len(in.Diagnostics) != 0 {
		t.Errorf("unloaded job: timer %q journal %q diagnostics %+v", in.Timer, in.Journal, in.Diagnostics)
	}
}
