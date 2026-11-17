// Package run wires Positron together and opens the window.
package run

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GabiHert/positron/internal/app"
	"github.com/GabiHert/positron/internal/assistant"
	"github.com/GabiHert/positron/internal/backend"
	"github.com/GabiHert/positron/internal/claude"
	"github.com/GabiHert/positron/internal/domain"
	"github.com/GabiHert/positron/internal/engine"
	"github.com/GabiHert/positron/internal/imagefile"
	"github.com/GabiHert/positron/internal/mcpserver"
	"github.com/GabiHert/positron/internal/media"
	"github.com/GabiHert/positron/internal/mediacapture"
	"github.com/GabiHert/positron/internal/menubar"
	"github.com/GabiHert/positron/internal/migrate"
	notifier "github.com/GabiHert/positron/internal/notify"
	"github.com/GabiHert/positron/internal/paths"
	"github.com/GabiHert/positron/internal/speech"
	"github.com/GabiHert/positron/internal/speechworker"
	"github.com/GabiHert/positron/internal/store"
	"github.com/GabiHert/positron/internal/system"
	"github.com/GabiHert/positron/internal/uigate"
	"github.com/GabiHert/positron/internal/update"
	"github.com/GabiHert/positron/internal/whatsapp"
	"github.com/google/uuid"
	wailsv2 "github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Version is set at build time.
var Version = "0.1.0"

// cachedEnv reads the login shell environment once; it is slow and rarely changes.
func cachedEnv() func() []string {
	var once sync.Once
	var env []string
	return func() []string {
		once.Do(func() { env = claude.LoginShellEnv() })
		return append([]string(nil), env...)
	}
}

// Run starts the app.
func Run(assets embed.FS) error {
	startPprof()
	// wails build runs this to list the bound methods. It must not open, create or move the real
	// data folder: give it a throwaway one, which also turns the Deputy migration off.
	if generatingBindings {
		dir, err := os.MkdirTemp("", "positron-bindings-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		if err := os.Setenv("POSITRON_HOME", dir); err != nil {
			return err
		}
	}
	// Positron was called Deputy: bring Deputy's data over once, before anything opens it.
	var moved migrate.Result
	if p, ok := migrate.Default(); ok {
		var err error
		if moved, err = p.Run(context.Background()); err != nil {
			if !errors.Is(err, migrate.ErrOldRunning) {
				err = fmt.Errorf("Positron could not bring over Deputy's data, so it did not start: %w", err)
			}
			system.Alert(err.Error())
			return err
		}
	}
	if err := os.MkdirAll(paths.Home(), 0o700); err != nil {
		return err
	}
	if f, err := os.OpenFile(paths.LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	}
	if moved.Moved {
		log.Printf("migrate: moved Deputy's data folder to %s", paths.Home())
	}
	if moved.MovedToken {
		log.Printf("migrate: moved Deputy's GitHub token in the keychain to %q", update.KeychainService)
	}
	st, err := store.Open(paths.DBPath())
	if err != nil {
		return err
	}
	if moved.LoginItem || (moved.Moved && st.Settings().OpenAtLogin) {
		if err := (system.LoginItem{}).Set(true); err != nil {
			log.Printf("migrate: open at login: %v", err)
		}
	}
	st.FailRunning()
	wa, err := whatsapp.Open(context.Background(), paths.WhatsAppDBPath())
	if err != nil {
		return err
	}
	env := cachedEnv()
	gitEnv := func() []string { return claude.CleanEnv(env()) }
	go env() // warm the cache off the startup path

	var wctx context.Context
	var ctxMu sync.Mutex
	getCtx := func() context.Context {
		ctxMu.Lock()
		defer ctxMu.Unlock()
		return wctx
	}
	// While the window is closed, Positron keeps running but sends the window no events; it
	// catches up with one "resume" event when it shows again.
	gate := uigate.New(func(event string, data any) {
		if c := getCtx(); c != nil {
			wruntime.EventsEmit(c, event, data)
		}
	}, app.EventNavigate, app.EventOpenChat)
	emit := gate.Emit
	show := func() {
		if c := getCtx(); c != nil {
			menubar.ShowDockIcon()
			wruntime.Show(c)
			wruntime.WindowShow(c)
			wruntime.WindowUnminimise(c)
			gate.SetVisible(true)
		}
	}
	// hide closes the window the way its red button does: the app hides and keeps running.
	hide := func() {
		if c := getCtx(); c != nil {
			wruntime.Hide(c)
		}
	}

	mediaStore := media.Store{Dir: paths.MediaDir()}
	// engineSpeech only checks model files here; Whisper itself runs in a helper process, which
	// hands all of its memory back when it exits.
	engineSpeech := speech.New(speech.Config{ModelsDir: paths.ModelsDir()})
	stt := &speechworker.Client{ReadyFn: engineSpeech.ReadyFor, Command: speechHelper(paths.ModelsDir())}
	detector := speech.NewDetector(paths.ModelsDir())
	models := speech.NewModelManager(paths.ModelsDir(), engineSpeech, detector)
	runner := claude.Runner{EnvFn: env}
	item := menubar.New()

	var notify func(title, body string)
	upd := update.New(update.Deps{Version: Version, AppName: "Positron", Repo: update.DefaultRepo, Env: gitEnv,
		Emit: emit, Dir: paths.UpdateDir(), LogPath: paths.UpdateLogPath(),
		Notify: func(title, body string) { notify(title, body) }, Quit: func() {
			if c := getCtx(); c != nil {
				wruntime.Quit(c)
			}
		}})
	upd.CleanLeftovers()

	var be *backend.Backend
	// Agents that need the owner notify apart from WhatsApp messages: their own switch, their own
	// group in Notification Center, and a click opens Activity.
	notify = func(title, body string) {
		s := st.Settings()
		if !s.Notify {
			return
		}
		if err := notifier.Post(notifier.Notification{ID: "agents:" + uuid.NewString(), Title: title, Body: body, Thread: "agents",
			Sound: s.NotifySound, UserInfo: map[string]string{"view": "activity"}}); err != nil {
			log.Printf("notify: %v", err)
		}
	}
	var notifyBlocked atomic.Bool
	var failedMu sync.Mutex
	failedSeen := map[string]bool{} // runs whose failure was announced, so an update does not repeat it
	eng := engine.New(engine.Deps{
		Store: st, WA: wa, Runner: runner, Media: mediaStore,
		Transcriber: media.Transcriber{Engine: stt, Detector: detector, TmpDir: os.TempDir()},
		Speaker:     media.Speaker{EnvFn: env, TmpDir: os.TempDir()},
		RepoEnv:     gitEnv,
		OnRun: func(r domain.Run) {
			if be == nil {
				return
			}
			emit(app.EventActivity, be.RunDTO(r))
			failedMu.Lock()
			announce := r.Status == domain.RunFailed && r.Request == "" && !failedSeen[r.ID]
			if announce {
				failedSeen[r.ID] = true
			} else if r.Status != domain.RunFailed {
				delete(failedSeen, r.ID)
			}
			failedMu.Unlock()
			if announce {
				name, chat := r.AgentID, r.ChatJID
				if a, err := st.Agent(r.AgentID); err == nil {
					name = a.Name
				}
				if c, err := st.Chat(r.ChatJID); err == nil && c.Name != "" {
					chat = c.Name
				}
				notify(name+" couldn't answer in "+chat, r.Error)
			}
			s := be.Status()
			emit(app.EventStatus, s)
			item.SetWaiting(s.Waiting)
		},
		OnChats: func() { emit(app.EventChats, nil) },
		Notify:  notify,
		OnMessage: func(m domain.Message, change string) {
			if be != nil {
				be.OnMessage(m, change)
			}
		},
		OnUnread: func(chat string) {
			if be != nil {
				be.OnUnread(chat)
			}
		},
		OnReceipt: func(chat string, ids []string, status string) {
			if be != nil {
				be.OnReceipt(chat, ids, status)
			}
		},
		OnHistory: func(kind string, added map[string]int) {
			if be != nil {
				be.OnHistory(kind, added)
			}
		},
	})
	be = backend.New(backend.Deps{Store: st, WA: wa, Engine: eng, Media: mediaStore, Models: models, Runner: runner,
		Env: gitEnv, Emit: emit, Version: Version, Login: system.LoginItem{}, Dock: menubar.SetHideDockWhenClosed,
		Update:  upd,
		Focused: menubar.Focused,
		NotifyChat: func(n backend.ChatNotification) {
			// One notification per chat: a new message replaces the chat's previous one.
			if err := notifier.Post(notifier.Notification{ID: "chat:" + n.Chat, Title: n.Title, Subtitle: n.Subtitle, Body: n.Body,
				Thread: "chat:" + n.Chat, Sound: n.Sound, Image: n.Image, UserInfo: map[string]string{"chat": n.Chat}}); err != nil {
				log.Printf("notify: %v", err)
			}
		},
		ClearNotification: func(id string) {
			if c := getCtx(); c != nil {
				_ = wruntime.RemoveDeliveredNotification(c, id)
			}
		},
		OnUnreadTotal: item.SetUnread,
		NotifyBlocked: notifyBlocked.Load,
	})

	qe := be.QueryEngine()
	asst, err := assistant.New(assistant.Deps{
		Store: st, Query: qe, Env: env, RepoEnv: gitEnv, Dir: filepath.Join(paths.Home(), "assistant"), Emit: emit,
		OwnerName: func() string { return wa.State().PushName }, Board: be, DefaultRepoPath: backend.DefaultRepoPath,
	})
	if err != nil {
		return err
	}
	be.SetV2(asst, qe)
	mcpSrv := mcpserver.New(be, mcpserver.Options{Home: paths.Home(), DefaultRepoPath: backend.DefaultRepoPath})
	be.SetMCP(mcpSrv)
	scheduleCtx, stopSchedule := context.WithCancel(context.Background())
	go be.RunBoardSchedule(scheduleCtx)
	// Check for a new release a minute after launch, then every six hours.
	go upd.Watch(scheduleCtx, time.Minute, 6*time.Hour)

	wa.OnState(func(whatsapp.State) { be.EmitStatus() })
	wa.SetTimerSource(func(chat string) (whatsapp.Timer, bool) {
		c, err := st.Chat(chat)
		if err != nil || !c.DisappearingKnown {
			return whatsapp.Timer{}, false
		}
		t := whatsapp.Timer{Known: true, Seconds: uint32(max(c.Disappearing, 0))}
		if !c.DisappearingAt.IsZero() {
			t.SetAt = c.DisappearingAt.Unix()
		}
		return t, true
	})
	wa.OnMessage(eng.HandleIncoming)
	wa.OnUpdate(be.HandleWAUpdate)
	wa.OnGroups(func(gs []whatsapp.Group) {
		for _, g := range gs {
			_, _ = st.UpdateChat(g.JID, func(c *domain.Chat) {
				c.JID, c.Name, c.Members, c.IsGroup = g.JID, g.Name, g.Members, true
			})
		}
		emit(app.EventChats, nil)
	})

	bound, setCtx := app.New(be, func(ctx context.Context, title string) (string, error) {
		return wruntime.OpenDirectoryDialog(ctx, wruntime.OpenDialogOptions{Title: title, CanCreateDirectories: true})
	})
	app.SetFilePicker(bound, func(ctx context.Context, title string) ([]string, error) {
		return wruntime.OpenMultipleFilesDialog(ctx, wruntime.OpenDialogOptions{Title: title})
	})
	files := system.FileHandler{MediaDir: paths.MediaDir(), RepoDir: func(id string) (string, bool) {
		a, err := st.Agent(id)
		return a.RepoPath, err == nil
	}}
	app.SetImageFiles(bound, app.ImageFiles{
		Resolve: files.Path,
		SaveDialog: func(ctx context.Context, name string) (string, error) {
			return wruntime.SaveFileDialog(ctx, wruntime.SaveDialogOptions{Title: "Save image", DefaultFilename: name, CanCreateDirectories: true})
		},
		Copy: imagefile.Pasteboard,
	})

	go func() {
		// Media cleanup once a day.
		for {
			mediaStore.Prune(st.Settings().MediaRetentionDays)
			time.Sleep(24 * time.Hour)
		}
	}()

	mediacapture.Allow()
	return wailsv2.Run(&options.App{
		Title:             "Positron",
		Width:             1280,
		Height:            820,
		MinWidth:          960,
		MinHeight:         620,
		BackgroundColour:  &options.RGBA{R: 251, G: 251, B: 248, A: 255},
		Mac:               &mac.Options{TitleBar: mac.TitleBarHiddenInset()},
		HideWindowOnClose: true,
		Menu:              appMenu(hide, func() { wruntime.WindowMinimise(getCtx()) }),
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               instanceID(),
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { show() },
		},
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: files,
		},
		OnStartup: func(ctx context.Context) {
			ctxMu.Lock()
			wctx = ctx
			ctxMu.Unlock()
			setCtx(ctx)
			if err := wruntime.InitializeNotifications(ctx); err == nil {
				// macOS asks once; after that it answers from what the person chose.
				go func() {
					ok, err := wruntime.RequestNotificationAuthorization(ctx)
					notifyBlocked.Store(!ok)
					log.Printf("notifications: allowed=%v err=%v", ok, err)
				}()
				wruntime.OnNotificationResponse(ctx, func(r wruntime.NotificationResult) {
					if r.Error != nil {
						return
					}
					if chat, ok := r.Response.UserInfo["chat"].(string); ok && chat != "" {
						show()
						emit(app.EventOpenChat, chat)
					} else if v, ok := r.Response.UserInfo["view"].(string); ok && v == "activity" {
						show()
						emit(app.EventNavigate, "activity")
					}
				})
			} else {
				log.Printf("notifications: %v", err)
			}
			item.Install(menubar.Handlers{
				Activity: func() { show(); emit(app.EventNavigate, "activity") },
				Open:     show,
				Quit:     func() { wruntime.Quit(ctx) },
				Inbox:    func() { show(); emit(app.EventNavigate, "inbox") },
			})
			menubar.SetHideDockWhenClosed(st.Settings().HideDockWhenClosed)
			menubar.WatchApp(menubar.AppHandlers{Reopen: show, Visible: func(v bool) {
				held := gate.Skipped()
				gate.SetVisible(v)
				log.Printf("window: visible=%v, %d events held while closed", v, held)
			}})
			item.SetWaiting(be.Status().Waiting)
			item.SetUnread(be.GetUnreadTotal())
			go func() {
				if err := wa.Start(); err != nil {
					log.Printf("whatsapp: %v", err)
				}
			}()
		},
		OnShutdown: func(ctx context.Context) {
			stopSchedule()
			_ = mcpSrv.SetEnabled(false)
			asst.Close()
			wa.Stop()
			eng.Close()
			stt.Unload()
			_ = st.Close()
		},
		Bind: []interface{}{bound},
	})
}

// instanceID is the single-instance lock. A separate POSITRON_HOME is a separate Positron, so it
// gets its own lock and can run next to the installed app.
func instanceID() string {
	if h := os.Getenv("POSITRON_HOME"); h != "" {
		return "tech.bhlabs.positron." + uuid.NewSHA1(uuid.NameSpaceURL, []byte(h)).String()[:8]
	}
	return "tech.bhlabs.positron"
}

// appMenu is the macOS menu bar menu: the app menu (Hide, Quit ⌘Q), Edit (copy and paste in the
// window), File with Close Window (⌘W), which hides the window and keeps Positron running, and
// Window.
func appMenu(closeWindow, minimise func()) *menu.Menu {
	m := menu.NewMenu()
	m.Append(menu.AppMenu())
	file := m.AddSubmenu("File")
	file.AddText("Close Window", keys.CmdOrCtrl("w"), func(*menu.CallbackData) { closeWindow() })
	m.Append(menu.EditMenu())
	win := m.AddSubmenu("Window")
	win.AddText("Minimize", keys.CmdOrCtrl("m"), func(*menu.CallbackData) { minimise() })
	return m
}

// speechHelper starts this binary as the speech helper.
func speechHelper(modelsDir string) func() *exec.Cmd {
	return func() *exec.Cmd {
		self, err := os.Executable()
		if err != nil {
			self = os.Args[0]
		}
		cmd := exec.Command(self, speechworker.Flag, modelsDir)
		cmd.Stderr = os.Stderr
		return cmd
	}
}

// SpeechWorker runs Whisper for the app over stdin and stdout until the app closes stdin.
func SpeechWorker(modelsDir string) error {
	e := speech.New(speech.Config{ModelsDir: modelsDir, IdleUnload: 24 * time.Hour})
	defer e.Unload()
	return speechworker.Serve(os.Stdin, os.Stdout, e.Transcribe)
}
