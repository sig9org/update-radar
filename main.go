package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sig9org/chatxgo/notify"
	"github.com/sig9org/update-radar/internal/appconfig"
	ciscostate "github.com/sig9org/update-radar/internal/ciscostate"
	"github.com/sig9org/update-radar/internal/debugx"
	"github.com/sig9org/update-radar/internal/diff"
	"github.com/sig9org/update-radar/internal/githubapi"
	githubstate "github.com/sig9org/update-radar/internal/githubstate"
	"github.com/sig9org/update-radar/internal/logx"
	"github.com/sig9org/update-radar/internal/model"
	"github.com/sig9org/update-radar/internal/monitor"
	"github.com/sig9org/update-radar/internal/notification"
	"github.com/sig9org/update-radar/internal/output"
	"github.com/sig9org/update-radar/internal/repoconfig"
	"github.com/sig9org/update-radar/internal/scraper"
	"github.com/sig9org/update-radar/internal/selfupdate"
	"github.com/sig9org/update-radar/internal/sentinel"
	"github.com/sig9org/update-radar/internal/siteconfig"
	"github.com/sig9org/update-radar/internal/version"
	"github.com/sig9org/update-radar/internal/webclient"
	webstate "github.com/sig9org/update-radar/internal/webstate"
	"gopkg.in/yaml.v3"
)

type options struct {
	config                                                       string
	debug, debugSet, dryrun, init, silent, update, version, help bool
}
type savedState struct {
	Cisco  ciscostate.File   `yaml:"cisco"`
	GitHub githubstate.State `yaml:"github"`
	Web    webstate.State    `yaml:"web"`
}

type profileMessage struct {
	Profile       string
	Message       notify.Message
	Software      string
	Notifications appconfig.Notification
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	var o options
	fs := flag.NewFlagSet("update-radar", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.config, "config", "", "configuration file (default: config.yml)")
	fs.StringVar(&o.config, "c", "", "configuration file (default: config.yml)")
	fs.BoolVar(&o.debug, "debug", false, "enable debug output")
	fs.BoolVar(&o.dryrun, "dryrun", false, "do not send notifications or update state")
	fs.BoolVar(&o.init, "init", false, "initialize state before checking")
	fs.BoolVar(&o.silent, "silent", false, "suppress normal output")
	fs.BoolVar(&o.update, "update", false, "update update-radar to the latest release")
	fs.BoolVar(&o.help, "h", false, "show help")
	fs.BoolVar(&o.help, "help", false, "show help")
	fs.BoolVar(&o.version, "v", false, "show version")
	fs.BoolVar(&o.version, "version", false, "show version")
	fs.Usage = func() { usage(stdout) }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "debug" {
			o.debugSet = true
		}
	})
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected arguments:", strings.Join(fs.Args(), " "))
		return 2
	}
	if o.help {
		usage(stdout)
		return 0
	}
	if o.version {
		fmt.Fprintln(stdout, version.Version)
		return 0
	}
	if o.update {
		msg, err := selfupdate.Update(context.Background(), version.Version)
		if err != nil {
			output.Fprintln(stderr, output.Error, err)
			return 1
		}
		fmt.Fprintln(stdout, msg)
		return 0
	}
	path, err := appconfig.Resolve(o.config)
	if err != nil {
		output.Fprintln(stderr, output.Error, err)
		return 1
	}
	cfg, err := appconfig.Load(path)
	if err != nil {
		output.Fprintln(stderr, output.Error, err)
		return 1
	}
	if !o.debugSet {
		o.debug = cfg.Settings.Debug
	}
	if o.silent && o.debug {
		o.silent = false
	}
	logger := &logx.Logger{Out: stdout, Err: stderr, Silent: o.silent, Debug: o.debug}
	debugx.Enable(o.debug)
	debugx.Writer = stdout
	logger.Debugf("loaded %s (%d profiles)", path, len(cfg.Profiles))
	if o.dryrun {
		logger.Infof("Dry run: would check %d profile(s); notifications and state writes are disabled.", len(cfg.Profiles))
		return 0
	}
	statePath := appconfig.StatePath(path)
	st := savedState{Cisco: ciscostate.File{Sites: map[string]model.Snapshot{}}, GitHub: githubstate.State{Repositories: map[string]githubstate.RepoState{}}, Web: webstate.State{Sites: map[string]webstate.SiteState{}}}
	if !o.init {
		if data, e := os.ReadFile(statePath); e == nil {
			if e = yaml.Unmarshal(data, &st); e != nil {
				logger.Errorf("parse state: %v", e)
				return 1
			}
		} else if !os.IsNotExist(e) {
			logger.Errorf("read state: %v", e)
			return 1
		}
	}
	ctx := context.Background()
	chatMessages := make([]profileMessage, 0)
	for name, p := range cfg.Profiles {
		logger.Infof("[%s] checking", name)
		logger.Debugf("profile=%s cisco=%d github=%d web=%d threads=%d timeout=%s headless=%t separate=%t user-agent=%t", name, len(p.Cisco), len(p.GitHub), len(p.Web), cfg.Settings.Threads, cfg.Settings.Timeout, cfg.Settings.Headless, cfg.Settings.Separate, strings.TrimSpace(cfg.Settings.UserAgent) != "")
		msgs, newSt, err := checkProfile(ctx, name, cfg.Settings, p, st, logger)
		if err != nil {
			logger.Errorf("profile %s: %v", name, err)
			st = newSt
			continue
		}
		st = newSt
		chatMessages = append(chatMessages, msgs...)
	}
	if err := saveState(statePath, st); err != nil {
		logger.Errorf("save state: %v", err)
		return 1
	}
	if len(chatMessages) == 0 {
		logger.Infof("No updates detected.")
		return 0
	}
	exitCode := 0
	for _, item := range chatMessages {
		if err := sendProfileMessage(ctx, item, logger); err != nil {
			exitCode = 1
		}
	}
	return exitCode
}

func sendProfileMessage(ctx context.Context, item profileMessage, logger *logx.Logger) error {
	tools := []string{"webex", "teams", "slack", "discord"}
	configured := false
	var firstErr error
	for _, tool := range tools {
		cfg, mentions, enabled := notificationConfig(item.Notifications, tool)
		if !enabled {
			continue
		}
		configured = true
		message := item.Message
		message.Mentions = mentions
		dispatcher, err := notify.NewDispatcher(cfg)
		if err != nil {
			logger.Errorf("notification setup for %s: %v", notificationToolName(tool), err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		results, err := dispatcher.Send(ctx, message)
		if err != nil {
			logger.Errorf("notification to %s: %v", notificationToolName(tool), err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, result := range results {
			if result.Err != nil {
				logger.Errorf("notification to %s: %v", notificationToolName(result.Tool), result.Err)
				if firstErr == nil {
					firstErr = result.Err
				}
				continue
			}
			if item.Software == "" {
				logger.Successf("[%s] Notification sent to %s", item.Profile, notificationToolName(result.Tool))
			} else {
				logger.Successf("[%s] Notification sent to %s for %s", item.Profile, notificationToolName(result.Tool), item.Software)
			}
		}
	}
	if !configured {
		return nil
	}
	return firstErr
}

func notificationConfig(n appconfig.Notification, tool string) (notify.Config, []notify.Mention, bool) {
	var cfg notify.Config
	var raw []string
	switch tool {
	case "webex":
		cfg = notify.Config{Proxy: n.Proxy, Webex: notify.WebexConfig{Token: n.Webex.Token, Dest: n.Webex.Destination}}
		raw = n.Webex.Mention
	case "teams":
		cfg = notify.Config{Proxy: n.Proxy, Teams: notify.TeamsConfig{Dest: n.Teams.Destination}}
		raw = n.Teams.Mention
	case "slack":
		cfg = notify.Config{Proxy: n.Proxy, Slack: notify.SlackConfig{Dest: n.Slack.Destination, Token: n.Slack.Token, Channel: n.Slack.Channel}}
		raw = n.Slack.Mention
	case "discord":
		cfg = notify.Config{Proxy: n.Proxy, Discord: notify.DiscordConfig{Dest: n.Discord.Destination}}
		raw = n.Discord.Mention
	}
	if cfg.Webex.Dest == "" && cfg.Teams.Dest == "" && cfg.Slack.Dest == "" && cfg.Discord.Dest == "" {
		return cfg, nil, false
	}
	mentions := make([]notify.Mention, 0, len(raw))
	for _, value := range raw {
		mention, err := notify.ParseMention(value)
		if err == nil {
			mentions = append(mentions, mention)
		}
	}
	return cfg, mentions, true
}

func notificationToolName(tool string) string {
	switch strings.ToLower(tool) {
	case "webex":
		return "Cisco Webex"
	case "teams":
		return "Microsoft Teams"
	case "slack":
		return "Slack"
	case "discord":
		return "Discord"
	default:
		return tool
	}
}

func notificationSoftware(software string) string {
	if software == "" {
		return ""
	}
	return " for " + software
}

func separateSoftware(settings appconfig.Settings, message notify.Message) string {
	if !settings.Separate {
		return ""
	}
	return message.Subject
}

func checkProfile(ctx context.Context, profile string, s appconfig.Settings, p appconfig.Profile, st savedState, logger *logx.Logger) ([]profileMessage, savedState, error) {
	var messages []profileMessage
	mentions := make([]notify.Mention, 0, len(s.Mention))
	for _, raw := range s.Mention {
		m, err := notify.ParseMention(raw)
		if err != nil {
			return nil, st, err
		}
		mentions = append(mentions, m)
	}
	if len(p.GitHub) > 0 {
		for _, repo := range p.GitHub {
			logger.Debugf("GitHub URL: %s", repo.URL)
		}
		cfg := repoconfig.Config{Settings: repoconfig.Settings{GitHubToken: s.GitHubToken}, Repositories: p.GitHub}
		events, next := monitor.CheckRepositories(ctx, githubapi.NewClient(s.GitHubToken), cfg, st.GitHub)
		st.GitHub = next
		for _, event := range events {
			logger.Warnf("GitHub version update: %s %s -> %s", event.RepoName, event.Previous, event.Current)
		}
		if len(events) > 0 {
			var m notify.Message
			var err error
			if s.Separate {
				var ms []notify.Message
				ms, err = monitor.BuildSeparateMessages(events, cfg.Settings)
				for _, message := range ms {
					messages = append(messages, profileMessage{Profile: profile, Message: message, Software: separateSoftware(s, message), Notifications: p.Notifications})
				}
			} else {
				m, err = monitor.BuildMessage(events, cfg.Settings)
				messages = append(messages, profileMessage{Profile: profile, Message: m, Software: separateSoftware(s, m), Notifications: p.Notifications})
			}
			if err != nil {
				return nil, st, err
			}
		}
	}
	if len(p.Web) > 0 {
		for _, site := range p.Web {
			logger.Debugf("Web URL: %s", site.Check)
		}
		cfg := siteconfig.Config{Sites: p.Web}
		events, next := sentinel.CheckSites(ctx, webclient.NewClient(s.Timeout), cfg, st.Web)
		st.Web = next
		for _, event := range events {
			logger.Warnf("Web content update: %s %s -> %s", event.SiteName, event.Previous, event.Current)
		}
		if len(events) > 0 {
			ms, err := sentinel.BuildMessages(events, cfg.Settings, s.Separate)
			if err != nil {
				return nil, st, err
			}
			for _, message := range ms {
				messages = append(messages, profileMessage{Profile: profile, Message: message, Software: separateSoftware(s, message), Notifications: p.Notifications})
			}
		}
	}
	if len(p.Cisco) > 0 {
		sites := make([]model.Site, len(p.Cisco))
		for i, x := range p.Cisco {
			sites[i] = model.Site{Name: x.Name, URL: x.URL}
		}
		timeout := s.Timeout
		if timeout == 0 {
			timeout = 45 * time.Second
		}
		pool, err := scraper.OpenPool(ctx, scraper.Options{Timeout: timeout, Headless: s.Headless, UserAgent: s.UserAgent}, s.Threads)
		if err != nil {
			return nil, st, err
		}
		defer pool.Close()
		type fetchResult struct {
			snapshot model.Snapshot
			err      error
		}
		fetched := make([]fetchResult, len(sites))
		workerCount := s.Threads
		if workerCount < 1 {
			workerCount = 1
		}
		if workerCount > len(sites) {
			workerCount = len(sites)
		}
		logger.Debugf("Cisco workers=%d sites=%d", workerCount, len(sites))
		jobs := make(chan int)
		var wg sync.WaitGroup
		for worker := 0; worker < workerCount; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for index := range jobs {
					site := sites[index]
					logger.Debugf("Cisco URL: %s", site.URL)
					snapshot, fetchErr := pool.Fetch(ctx, site)
					fetched[index] = fetchResult{snapshot: snapshot, err: fetchErr}
				}
			}()
		}
		for index := range sites {
			jobs <- index
		}
		close(jobs)
		wg.Wait()
		results := make([]model.SiteDiff, 0)
		for index, site := range sites {
			snap, e := fetched[index].snapshot, fetched[index].err
			if e != nil {
				logger.Errorf("[%s] Cisco check failed for %s: %v", profile, site.Name, e)
				continue
			}
			prev, ok := st.Cisco.Sites[site.URL]
			var pp *model.Snapshot
			if ok {
				pp = &prev
			}
			d := diff.Compute(site, pp, snap)
			logger.Debugf("Cisco version: %s suggested=%v latest=%v", site.Name, snap.Suggested, snap.Latest)
			if d.Changed() {
				changes := make([]string, 0, 2)
				if d.Suggested.Changed() {
					changes = append(changes, fmt.Sprintf("suggested: %s -> %s", strings.Join(d.Suggested.Removed, ","), strings.Join(d.Suggested.Added, ",")))
				}
				if d.Latest.Changed() {
					changes = append(changes, fmt.Sprintf("latest: %s -> %s", strings.Join(d.Latest.Removed, ","), strings.Join(d.Latest.Added, ",")))
				}
				logger.Warnf("Cisco version update: %s (%s)", site.Name, strings.Join(changes, "; "))
			}
			st.Cisco.Sites[site.URL] = snap
			if d.Changed() {
				results = append(results, d)
			}
		}
		for _, message := range notification.Messages(results, time.Now(), s.Separate, mentions...) {
			messages = append(messages, profileMessage{Profile: profile, Message: message, Software: separateSoftware(s, message), Notifications: p.Notifications})
		}
	}
	return messages, st, nil
}

func chatConfig(profiles map[string]appconfig.Profile) (notify.Config, error) {
	var c notify.Config
	for _, p := range profiles {
		if c.Proxy == "" {
			c.Proxy = p.Notifications.Proxy
		}
		if c.Teams.Dest == "" {
			c.Teams = notify.TeamsConfig{Dest: p.Notifications.Teams.Destination}
		}
		if c.Webex.Dest == "" {
			c.Webex = notify.WebexConfig{Token: p.Notifications.Webex.Token, Dest: p.Notifications.Webex.Destination}
		}
		if c.Slack.Dest == "" {
			c.Slack = notify.SlackConfig{Dest: p.Notifications.Slack.Destination, Token: p.Notifications.Slack.Token, Channel: p.Notifications.Slack.Channel}
		}
		if c.Discord.Dest == "" {
			c.Discord = notify.DiscordConfig{Dest: p.Notifications.Discord.Destination}
		}
	}
	return c, nil
}
func saveState(path string, st savedState) error {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	err := encoder.Encode(st)
	closeErr := encoder.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0600)
}
func usage(w io.Writer) {
	fmt.Fprintln(w, "update-radar "+version.Version)
	fmt.Fprintln(w, "\nUsage of update-radar:")
	fmt.Fprintln(w, "  -c, -config <path>  path to the config file")
	fmt.Fprintln(w, "      -debug          print detailed debug information")
	fmt.Fprintln(w, "      -dryrun         do not notify or save state")
	fmt.Fprintln(w, "  -h, -help           show this help message")
	fmt.Fprintln(w, "      -init           initialize the state file before checking")
	fmt.Fprintln(w, "      -silent         suppress standard output (overridden by -debug)")
	fmt.Fprintln(w, "      -update         update update-radar itself to the latest release")
	fmt.Fprintln(w, "  -v, -version        print the update-radar version")
}
