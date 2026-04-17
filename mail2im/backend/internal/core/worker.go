package core

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	appconfig "mail2im/internal/config"
	"mail2im/internal/models"
	"mime"
	"regexp"
	"strings"
	"time"

	imap "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/mail"
	nanoid "github.com/matoous/go-nanoid/v2"
)

type Worker struct {
	Account         models.Account
	Client          *imapclient.Client
	StopChan        chan struct{}
	IsRunning       bool
	SupportsIDLE    bool
	Capabilities    []string
	SecurityMode    string
	Mailboxes       []models.Mailbox
	lastMailboxSync time.Time
	Silent          bool
	idleUpdates     chan idleEvent
}

type idleEvent struct {
	kind    string
	seqNum  uint32
	mailbox *imapclient.UnilateralDataMailbox
}

type ConnectionInfo struct {
	Capabilities []string
	SupportsIDLE bool
	SecurityMode string
}

func decodeMimeWord(val string) string {
	dec := new(mime.WordDecoder)
	s, err := dec.DecodeHeader(val)
	if err != nil {
		return val
	}
	return s
}

func formatAddresses(addrs []imap.Address) string {
	var parts []string
	for _, a := range addrs {
		addr := fmt.Sprintf("%s@%s", a.Mailbox, a.Host)
		name := strings.TrimSpace(decodeMimeWord(a.Name))
		if name != "" {
			parts = append(parts, fmt.Sprintf("%s <%s>", name, addr))
		} else {
			parts = append(parts, addr)
		}
	}
	return strings.Join(parts, ", ")
}

func NewWorker(account models.Account) *Worker {
	return &Worker{
		Account:  account,
		StopChan: make(chan struct{}),
	}
}

func (w *Worker) log(level string, state WorkerState, msg string) {
	if w.Silent {
		return
	}
	Debug.Log(w.Account.ID, w.Account.Email, level, state, msg)
}

func isNoSelect(mb models.Mailbox) bool {
	attrs := strings.ToLower(mb.Attributes)
	return strings.Contains(attrs, "\\noselect")
}

func is163Server(host string) bool {
	h := strings.ToLower(host)
	return strings.Contains(h, "163.com") || strings.Contains(h, "126.com") || strings.Contains(h, "yeah.net") || strings.Contains(h, "yeah.com")
}

func (w *Worker) pushIdleEvent(ev idleEvent) {
	if w.idleUpdates == nil {
		return
	}

	select {
	case w.idleUpdates <- ev:
	default:
		w.log("warn", StateIDLE, "IDLE update channel full, dropping event")
	}
}

func sendIMAPID(conn *imapclient.Client) error {
	if conn == nil {
		return fmt.Errorf("imap client is nil")
	}
	if caps := conn.Caps(); caps != nil && !caps.Has(imap.CapID) {
		return fmt.Errorf("server does not support ID")
	}
	_, err := conn.ID(&imap.IDData{
		Name:    "Mail2IM",
		Version: "1.0.0",
		Vendor:  "Mail2IM",
	}).Wait()
	return err
}

func (w *Worker) MarkAsRead(uid uint) error {
	if w.Client == nil {
		return fmt.Errorf("client not connected")
	}

	// 1. Find which mailbox this UID belongs to
	var email models.Email
	if err := DB.Where("account_id = ? AND uid = ?", w.Account.ID, uid).First(&email).Error; err != nil {
		return fmt.Errorf("email not found locally")
	}

	var mailbox models.Mailbox
	if err := DB.Where("account_id = ? AND name = ?", w.Account.ID, email.Mailbox).First(&mailbox).Error; err != nil {
		// Try raw path if name fails (fallback)
		if err := DB.Where("account_id = ? AND path = ?", w.Account.ID, email.MailboxPath).First(&mailbox).Error; err != nil {
			return fmt.Errorf("mailbox not found")
		}
	}

	// 2. Select Mailbox
	if _, err := w.Client.Select(mailbox.Path, nil).Wait(); err != nil {
		return fmt.Errorf("failed to select mailbox: %v", err)
	}

	// 3. Store flags
	uidSet := imap.UIDSet{}
	uidSet.AddNum(imap.UID(uid))

	storeFlags := imap.StoreFlags{
		Op:    imap.StoreFlagsAdd,
		Flags: []imap.Flag{imap.FlagSeen},
	}

	storeCmd := w.Client.Store(uidSet, &storeFlags, nil)
	if err := storeCmd.Close(); err != nil {
		return fmt.Errorf("failed to store flags: %v", err)
	}

	// 4. Update local DB
	DB.Model(&email).Update("is_read", true)

	w.log("info", StateIDLE, fmt.Sprintf("Marked email UID %d as read", uid))
	return nil
}

func (w *Worker) Start() {
	if w.IsRunning {
		return
	}
	w.IsRunning = true
	w.idleUpdates = make(chan idleEvent, 16)
	w.log("info", StateConnecting, "Worker starting")
	go w.run()
}

func (w *Worker) Stop() {
	if !w.IsRunning {
		return
	}
	close(w.StopChan)
	w.IsRunning = false
	if w.Client != nil {
		if err := w.Client.Logout().Wait(); err != nil {
			w.log("warn", StateDisconnected, fmt.Sprintf("Logout error: %v", err))
		}
		w.Client.Close()
	}
	w.log("info", StateDisconnected, "Worker stopped")
}

func (w *Worker) dialAndLogin() (*imapclient.Client, *ConnectionInfo, error) {
	addr := fmt.Sprintf("%s:%d", w.Account.IMAPServer, w.Account.IMAPPort)
	dialer := NewProxyDialer(w.Account.Proxy)
	rawConn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, nil, err
	}

	mode := strings.ToLower(w.Account.SSLMode)
	if mode == "" {
		if w.Account.UseSSL {
			mode = "ssl"
		} else {
			mode = "none"
		}
	}

	if w.idleUpdates == nil {
		w.idleUpdates = make(chan idleEvent, 16)
	}

	opts := &imapclient.Options{
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Expunge: func(seqNum uint32) {
				w.pushIdleEvent(idleEvent{kind: "expunge", seqNum: seqNum})
			},
			Mailbox: func(data *imapclient.UnilateralDataMailbox) {
				w.pushIdleEvent(idleEvent{kind: "mailbox", mailbox: data})
			},
		},
	}

	info := &ConnectionInfo{SecurityMode: mode}
	tlsConfig := &tls.Config{ServerName: w.Account.IMAPServer}
	var c *imapclient.Client
	switch mode {
	case "ssl":
		tlsConn := tls.Client(rawConn, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			return nil, nil, err
		}
		c = imapclient.New(tlsConn, opts)
	case "starttls":
		optsWithTLS := *opts
		optsWithTLS.TLSConfig = tlsConfig
		c, err = imapclient.NewStartTLS(rawConn, &optsWithTLS)
		if err != nil {
			return nil, nil, fmt.Errorf("starttls failed: %w", err)
		}
		info.SecurityMode = "starttls"
	default:
		c = imapclient.New(rawConn, opts)
	}

	password := w.Account.Password
	if decrypted, err := Decrypt(password); err == nil && decrypted != "" {
		password = decrypted
	}
	login := w.Account.Login
	if login == "" {
		login = w.Account.Email
	}

	if err := c.WaitGreeting(); err != nil {
		return nil, nil, err
	}

	if err := c.Login(login, password).Wait(); err != nil {
		c.Close()
		return nil, nil, err
	}

	if is163Server(w.Account.IMAPServer) {
		if err := sendIMAPID(c); err != nil {
			w.log("warn", StateConnecting, fmt.Sprintf("163 IMAP ID failed: %v", err))
		} else {
			w.log("info", StateConnecting, "163 IMAP ID sent")
		}
	}

	if caps := c.Caps(); caps != nil {
		for cap := range caps {
			info.Capabilities = append(info.Capabilities, string(cap))
		}
		info.SupportsIDLE = caps.Has(imap.CapIdle) || caps.Has(imap.CapIMAP4rev2)
	}

	return c, info, nil
}

func (w *Worker) connect() error {
	w.idleUpdates = make(chan idleEvent, 16)
	conn, info, err := w.dialAndLogin()
	if err != nil {
		return err
	}
	w.Client = conn
	if info != nil {
		w.SupportsIDLE = info.SupportsIDLE
		w.Capabilities = info.Capabilities
		w.SecurityMode = info.SecurityMode
	}
	return nil
}

func (w *Worker) run() {
	defer func() {
		w.IsRunning = false
		if !w.Silent {
			Debug.UpdateWorkerStatus(w.Account.ID, w.Account.Email, StateDisconnected, "Stopped")
		}
	}()

	for {
		select {
		case <-w.StopChan:
			return
		default:
			if err := w.connect(); err != nil {
				w.log("error", StateError, fmt.Sprintf("Connection failed: %v", err))
				time.Sleep(30 * time.Second)
				continue
			}

			w.log("info", StateConnecting, "Connected")

			w.syncMailboxes()

			// Determine Watch List
			var idleTarget *models.Mailbox
			var pollTargets []models.Mailbox

			for i := range w.Mailboxes {
				mb := w.Mailboxes[i]
				if mb.WatchMode == "none" {
					continue
				}

				if idleTarget == nil && w.Account.UseIDLE && w.SupportsIDLE && (mb.WatchMode == "idle" || mb.Type == "primary") {
					idleTarget = &w.Mailboxes[i]
				} else {
					pollTargets = append(pollTargets, mb)
				}
			}

			// If no explicit IDLE target found but we support IDLE and have INBOX, fallback to INBOX
			if idleTarget == nil && w.Account.UseIDLE && w.SupportsIDLE {
				for i := range w.Mailboxes {
					if strings.EqualFold(w.Mailboxes[i].Name, "INBOX") {
						idleTarget = &w.Mailboxes[i]
						// Remove from pollTargets if present
						// (Simplified: just rebuild pollTargets to be safe or accept double check)
						newPoll := []models.Mailbox{}
						for _, p := range pollTargets {
							if p.ID != idleTarget.ID {
								newPoll = append(newPoll, p)
							}
						}
						pollTargets = newPoll
						break
					}
				}
			}

			initialNew := 0
			if idleTarget != nil {
				initialNew += w.fetchMailboxMessages(idleTarget)
			}
			for i := range pollTargets {
				initialNew += w.fetchMailboxMessages(&pollTargets[i])
			}
			if initialNew > 0 {
				w.log("info", StatePolling, fmt.Sprintf("Fetched %d new messages", initialNew))
			}

			if idleTarget != nil {
				w.runIDLE(idleTarget, pollTargets)
			} else {
				// Add idleTarget to pollTargets if it was nil (logic above ensures strictly separated, but if IDLE failed we treat all as poll)
				// Actually if idleTarget is nil here, it means we are in pure poll mode.
				// We need to make sure we poll everything.
				// The logic above: if idleTarget is nil, everything went to pollTargets (except if we missed logic).
				// Re-scan to be sure:
				allPoll := []models.Mailbox{}
				for i := range w.Mailboxes {
					if w.Mailboxes[i].WatchMode != "none" {
						allPoll = append(allPoll, w.Mailboxes[i])
					}
				}
				w.runPolling(allPoll)
			}
		}
	}
}

func (w *Worker) runIDLE(home *models.Mailbox, others []models.Mailbox) {
	w.log("info", StateIDLE, fmt.Sprintf("Entering IDLE mode on %s", home.Name))

	pollInterval := w.resolvePollInterval()
	idleRefresh := 5 * time.Minute

	if err := w.selectMailbox(home.Path); err != nil {
		w.log("error", StateError, fmt.Sprintf("Select %s failed: %v", home.Name, err))
		return
	}

	startIdle := func() (*imapclient.IdleCommand, chan error, bool) {
		idleCmd, err := w.Client.Idle()
		if err != nil {
			w.log("error", StateError, fmt.Sprintf("Start IDLE failed: %v", err))
			return nil, nil, false
		}
		done := make(chan error, 1)
		go func() {
			done <- idleCmd.Wait()
		}()
		w.log("debug", StateIDLE, "Started IDLE session")
		return idleCmd, done, true
	}

	stopIdle := func(idleCmd *imapclient.IdleCommand, done chan error, reason string) {
		if idleCmd == nil {
			return
		}
		if err := idleCmd.Close(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
			w.log("warn", StateIDLE, fmt.Sprintf("Stop IDLE (%s) error: %v", reason, err))
		}
		// Wait for IDLE to finish
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			w.log("warn", StateIDLE, "IDLE close timeout")
		}
	}

	idleCmd, idleDone, ok := startIdle()
	if !ok {
		return
	}

	pollTicker := time.NewTicker(pollInterval)
	idleTimer := time.NewTimer(idleRefresh)
	defer pollTicker.Stop()
	defer idleTimer.Stop()

	resetTimer := func() {
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(idleRefresh)
	}

	for {
		select {
		case <-w.StopChan:
			stopIdle(idleCmd, idleDone, "stop")
			return
		case update := <-w.idleUpdates:
			w.log("debug", StateIDLE, fmt.Sprintf("IDLE update: %v", update.kind))
			stopIdle(idleCmd, idleDone, "update")

			// Fetch Home
			w.fetchMailboxMessages(home)

			// Resume IDLE
			if err := w.selectMailbox(home.Path); err != nil {
				return
			}
			idleCmd, idleDone, ok = startIdle()
			if !ok {
				return
			}
			resetTimer()

		case <-pollTicker.C:
			stopIdle(idleCmd, idleDone, "poll")

			// Poll others
			if len(others) > 0 {
				w.log("debug", StatePolling, "Polling other mailboxes...")
				for i := range others {
					w.fetchMailboxMessages(&others[i])
				}
			}

			// Also check Home again just in case
			w.fetchMailboxMessages(home)

			// Update interval
			next := w.resolvePollInterval()
			if next != pollInterval {
				pollInterval = next
				pollTicker.Reset(pollInterval)
			}

			// Resume IDLE
			if err := w.selectMailbox(home.Path); err != nil {
				return
			}
			idleCmd, idleDone, ok = startIdle()
			if !ok {
				return
			}
			resetTimer()

		case <-idleTimer.C:
			stopIdle(idleCmd, idleDone, "refresh")
			if err := w.selectMailbox(home.Path); err != nil {
				return
			}
			idleCmd, idleDone, ok = startIdle()
			if !ok {
				return
			}
			resetTimer()

		case err := <-idleDone:
			if err != nil {
				w.log("warn", StateIDLE, fmt.Sprintf("IDLE dropped: %v", err))
				return // Reconnect
			}
		}
	}
}

func (w *Worker) runPolling(targets []models.Mailbox) {
	if len(targets) == 0 {
		w.log("warn", StatePolling, "No mailboxes to poll")
		time.Sleep(time.Minute)
		return
	}

	interval := w.resolvePollInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		w.log("debug", StatePolling, fmt.Sprintf("Polling %d mailboxes", len(targets)))
		total := 0
		for i := range targets {
			total += w.fetchMailboxMessages(&targets[i])
		}
		if total > 0 {
			w.log("info", StatePolling, fmt.Sprintf("Fetched %d new messages", total))
		}

		select {
		case <-w.StopChan:
			return
		case <-ticker.C:
			next := w.resolvePollInterval()
			if next != interval {
				interval = next
				ticker.Reset(interval)
			}
		}
	}
}

func (w *Worker) resolvePollInterval() time.Duration {
	day := w.Account.PollIntervalDay
	if day <= 0 {
		day = 60
	}
	night := w.Account.PollIntervalNight
	if night <= 0 {
		night = day
	}

	window := GetNightWindow()
	loc := GetSystemLocation()
	now := time.Now().In(loc)
	if window.Enabled && IsInWindow(now, window, loc) {
		return time.Duration(night) * time.Second
	}
	return time.Duration(day) * time.Second
}

func (w *Worker) selectMailbox(target string) error {
	if w.Client == nil {
		return fmt.Errorf("client is not connected")
	}

	path := target
	for _, mb := range w.Mailboxes {
		if strings.EqualFold(mb.Name, target) || mb.Path == target {
			path = mb.Path
			break
		}
	}

	if _, err := w.Client.Select(path, nil).Wait(); err != nil {
		return err
	}
	return nil
}

func (w *Worker) fetchMailboxByName(name string) int {
	for i := range w.Mailboxes {
		if strings.EqualFold(w.Mailboxes[i].Name, name) || w.Mailboxes[i].Path == name {
			return w.fetchMailboxMessages(&w.Mailboxes[i])
		}
	}
	return 0
}

func (w *Worker) fetchAllMailboxes() int {
	if w.Client == nil {
		return 0
	}

	if len(w.Mailboxes) == 0 || time.Since(w.lastMailboxSync) > 10*time.Minute {
		w.syncMailboxes()
	}

	if len(w.Mailboxes) == 0 {
		w.log("warn", StatePolling, "No mailboxes available to sync")
		return 0
	}

	totalNew := 0
	for i := range w.Mailboxes {
		totalNew += w.fetchMailboxMessages(&w.Mailboxes[i])
	}

	DB.Model(&models.Account{}).Where("id = ?", w.Account.ID).Update("last_sync_at", time.Now())
	return totalNew
}

func (w *Worker) syncMailboxes() {
	if w.Client == nil {
		return
	}

	w.log("debug", StateConnecting, "Syncing mailboxes")

	listCmd := w.Client.List("", "*", nil)
	var synced []models.Mailbox
	foundPaths := make(map[string]bool)

	for {
		mbox := listCmd.Next()
		if mbox == nil {
			break
		}

		name := mbox.Mailbox
		var attrsList []string
		for _, attr := range mbox.Attrs {
			attrsList = append(attrsList, string(attr))
		}
		attrs := strings.Join(attrsList, ",")
		foundPaths[mbox.Mailbox] = true

		var existing models.Mailbox
		if err := DB.Where("account_id = ? AND path = ?", w.Account.ID, mbox.Mailbox).First(&existing).Error; err == nil {
			existing.Name = name
			existing.Delimiter = string(mbox.Delim)
			existing.Attributes = attrs
			existing.WatchStatus = "verified"

			// Only auto-update type if it was "unknown" to preserve manual overrides?
			// Or always re-evaluate if not manually locked? (For now, let's assume simple overwrite if unknown)
			if existing.Type == "" || existing.Type == "unknown" {
				existing.Type = classifyMailboxType(w.Account.Provider, name, mbox.Mailbox)
			}
			DB.Save(&existing)
			synced = append(synced, existing)
			continue
		}

		// New Mailbox
		predictedType := classifyMailboxType(w.Account.Provider, name, mbox.Mailbox)
		watchMode := "none"
		if predictedType == "primary" {
			watchMode = "idle"
		}

		entry := models.Mailbox{
			AccountID:   w.Account.ID,
			Name:        name,
			Path:        mbox.Mailbox,
			Delimiter:   string(mbox.Delim),
			Attributes:  attrs,
			WatchStatus: "verified",
			Type:        predictedType,
			WatchMode:   watchMode,
		}
		DB.Create(&entry)
		synced = append(synced, entry)
	}

	if err := listCmd.Close(); err != nil {
		w.log("warn", StateError, fmt.Sprintf("List mailboxes failed: %v", err))
		return
	}

	// Handle deletions: Delete mailboxes in DB that are not in foundPaths
	// This preserves classification rules if they are stored elsewhere, but Mailbox record holds the state.
	// User requirement: "delete it".
	// But we might lose "LastUID". Re-creating it later will reset sync.
	// If we delete, we lose manual WatchMode settings.
	// Maybe just mark as "deleted"?
	// User said: "客户端这边也需要删除(但是分类策略可以保留)"
	// Since "Type" and "WatchMode" are on the Mailbox record, deleting it loses the strategy.
	// To support "strategy retention", we would need a separate "MailboxConfig" table keyed by Name/Path.
	// But given current structure, hard delete is requested.
	// I will perform hard delete for now as requested.
	if len(foundPaths) > 0 {
		DB.Where("account_id = ? AND path NOT IN ?", w.Account.ID, getMapKeys(foundPaths)).Delete(&models.Mailbox{})
	} else {
		// If foundPaths empty (rare/error?), don't delete everything blindly without verification.
		// But if List returned 0 items successfully...
	}

	w.Mailboxes = synced
	w.lastMailboxSync = time.Now()
}

func getMapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return []string{""} // Avoid empty IN clause error
	}
	return keys
}

func (w *Worker) fetchMailboxMessages(mb *models.Mailbox) int {
	if w.Client == nil {
		return 0
	}

	if isNoSelect(*mb) {
		w.log("debug", StatePolling, fmt.Sprintf("Skip noselect mailbox %s", mb.Path))
		return 0
	}

	if _, err := w.Client.Select(mb.Path, nil).Wait(); err != nil {
		w.log("warn", StateError, fmt.Sprintf("Select %s failed: %v", mb.Path, err))
		return 0
	}

	startUID := imap.UID(mb.LastUID + 1)
	if mb.LastUID == 0 && (strings.EqualFold(mb.Name, "inbox") || strings.EqualFold(mb.Path, "INBOX")) && w.Account.LastUID > 0 {
		startUID = imap.UID(w.Account.LastUID + 1)
	}
	if startUID == 0 {
		startUID = 1
	}
	uidSet := imap.UIDSet{}
	uidSet.AddRange(startUID, 0)

	criteria := &imap.SearchCriteria{
		UID:     []imap.UIDSet{uidSet},
		NotFlag: []imap.Flag{imap.FlagSeen},
	}

	searchRes, err := w.Client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		w.log("error", StateError, fmt.Sprintf("Search error in %s: %v", mb.Path, err))
		return 0
	}

	uids := searchRes.AllUIDs()
	if len(uids) == 0 {
		return 0
	}

	var fetchSet imap.UIDSet
	fetchSet.AddNum(uids...)

	bodySection := &imap.FetchItemBodySection{}
	fetchOptions := &imap.FetchOptions{
		UID:          true,
		Envelope:     true,
		InternalDate: true,
		Flags:        true,
		BodySection:  []*imap.FetchItemBodySection{bodySection},
	}

	fetchCmd := w.Client.Fetch(fetchSet, fetchOptions)

	var maxUID imap.UID
	newCount := 0
	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}

		buf, err := msg.Collect()
		if err != nil {
			w.log("error", StateError, fmt.Sprintf("Fetch collect error in %s: %v", mb.Path, err))
			continue
		}

		if buf.UID > maxUID {
			maxUID = buf.UID
		}

		if w.processFetchedMessage(buf, bodySection, *mb) {
			newCount++
		}
	}

	if err := fetchCmd.Close(); err != nil {
		w.log("warn", StateError, fmt.Sprintf("Fetch error in %s: %v", mb.Path, err))
	}

	if maxUID > 0 && uint(maxUID) > mb.LastUID {
		mb.LastUID = uint(maxUID)
		DB.Model(&models.Mailbox{}).Where("id = ?", mb.ID).Update("last_uid", mb.LastUID)
		if strings.EqualFold(mb.Name, "inbox") || strings.EqualFold(mb.Path, "INBOX") {
			DB.Model(&models.Account{}).Where("id = ?", w.Account.ID).Update("last_uid", mb.LastUID)
		}
	}

	if newCount > 0 {
		w.log("info", StatePolling, fmt.Sprintf("Fetched %d new messages from %s", newCount, mb.Name))
	}
	return newCount
}

func (w *Worker) processFetchedMessage(buf *imapclient.FetchMessageBuffer, section *imap.FetchItemBodySection, mailbox models.Mailbox) bool {
	if buf == nil {
		return false
	}

	body := buf.FindBodySection(section)
	if body == nil {
		w.log("warn", StateError, "Server didn't return message body")
		return false
	}

	mailType := mailbox.Type
	if mailType == "" {
		mailType = classifyMailboxType(w.Account.Provider, mailbox.Name, mailbox.Path)
	}
	priority := getPriorityByType(mailType)

	if buf.UID > 0 {
		var count int64
		DB.Model(&models.Email{}).Where("account_id = ? AND uid = ? AND mailbox_path = ?", w.Account.ID, buf.UID, mailbox.Path).Count(&count)
		if count > 0 {
			return false
		}
	}

	mr, err := mail.CreateReader(bytes.NewReader(body))
	if err != nil {
		w.log("error", StateError, fmt.Sprintf("Failed to create mail reader: %v", err))
		return false
	}

	env := buf.Envelope
	subject := ""
	messageID := ""
	fromStr := ""
	toStr := ""
	receivedAt := buf.InternalDate

	if env != nil {
		subject = decodeMimeWord(env.Subject)
		messageID = env.MessageID
		fromStr = formatAddresses(env.From)
		toStr = formatAddresses(env.To)
		if receivedAt.IsZero() && !env.Date.IsZero() {
			receivedAt = env.Date
		}
	}

	// Fallback to headers if envelope missing
	if subject == "" {
		if hdrSubject, _ := mr.Header.Text("Subject"); hdrSubject != "" {
			subject = decodeMimeWord(hdrSubject)
		}
	}
	if messageID == "" {
		messageID, _ = mr.Header.Text("Message-ID")
	}
	if fromStr == "" {
		if from, _ := mr.Header.AddressList("From"); len(from) > 0 {
			fromStr = decodeMimeWord(from[0].String())
		}
	}
	if toStr == "" {
		if to, _ := mr.Header.AddressList("To"); len(to) > 0 {
			toStr = decodeMimeWord(to[0].String())
		}
	}

	subject = decodeMimeWord(subject)
	fromStr = decodeMimeWord(fromStr)
	toStr = decodeMimeWord(toStr)

	var textBody, htmlBody string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			log.Printf("Failed to read part: %v", err)
			break
		}

		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			b, _ := io.ReadAll(p.Body)
			contentType, _, _ := h.ContentType()
			if contentType == "text/plain" {
				textBody = string(b)
			} else if contentType == "text/html" {
				htmlBody = string(b)
			}
		case *mail.AttachmentHeader:
			// TODO: handle attachment meta if needed
		}
	}

	pid, _ := nanoid.New()

	email := models.Email{
		ID:          pid,
		AccountID:   w.Account.ID,
		MessageID:   messageID,
		MailboxID:   mailbox.ID,
		UID:         uint(buf.UID),
		SeqNum:      uint(buf.SeqNum),
		Mailbox:     mailbox.Name,
		MailboxPath: mailbox.Path,
		MailType:    mailType,
		From:        fromStr,
		To:          toStr,
		Subject:     subject,
		TextBody:    textBody,
		HTMLBody:    htmlBody,
		ReceivedAt:  receivedAt,
		IsRead:      false,
	}
	if email.ReceivedAt.IsZero() {
		email.ReceivedAt = time.Now()
	}

	if err := DB.Create(&email).Error; err != nil {
		w.log("error", StateError, fmt.Sprintf("Failed to save email: %v", err))
		return false
	}

	RecordForwardLog(w.Account.ID, "receive", "received", nil, "", messageID, subject, fromStr, email.ReceivedAt, int(priority), "", "", "")

	Bus.Publish(Event{
		Type:     EventEmailReceived,
		Priority: priority,
		Source:   fmt.Sprintf("account:%d", w.Account.ID),
		Payload: map[string]interface{}{
			"uid":         buf.UID,
			"email_id":    email.ID,
			"public_id":   pid,
			"message_id":  messageID,
			"subject":     subject,
			"from":        fromStr,
			"mailbox":     mailbox.Name,
			"mailboxRaw":  mailbox.Path,
			"mailbox_id":  mailbox.ID,
			"mail_type":   mailType,
			"account_id":  w.Account.ID,
			"received_at": email.ReceivedAt,
		},
	})
	return true
}

// classifyMailboxType determines mail type based on mailbox metadata.
// Priority: 1) Provider folder mapping  2) FolderRule regex  3) "unknown"
func classifyMailboxType(providerID, name, path string) string {
	// 1. Provider folder mapping (exact match from providers.json)
	if providerID != "" {
		if t := appconfig.LookupFolderType(providerID, name, path); t != "" {
			return t
		}
	}

	// 2. FolderRule regex (user-configurable rules from DB)
	var rules []models.FolderRule
	if err := DB.Order("`order` asc").Find(&rules).Error; err != nil {
		log.Printf("Failed to load classification rules: %v", err)
		return "unknown"
	}

	for _, rule := range rules {
		if matched, _ := regexp.MatchString(rule.Pattern, path); matched {
			return rule.TargetType
		}
		if matched, _ := regexp.MatchString(rule.Pattern, name); matched {
			return rule.TargetType
		}
	}

	return "unknown"
}

func getPriorityByType(t string) EventPriority {
	var mt models.MailType
	if err := DB.Where("key = ?", t).First(&mt).Error; err != nil {
		// Fallback if type not found in DB
		return PriorityNormal
	}

	// Map int priority to EventPriority enum
	// 0=Low, 10=Normal, 20=High, 30=Critical
	switch {
	case mt.Priority >= 30:
		return PriorityCritical
	case mt.Priority >= 20:
		return PriorityHigh
	case mt.Priority >= 10:
		return PriorityNormal
	default:
		return PriorityLow
	}
}
