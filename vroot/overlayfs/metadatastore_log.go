package overlayfs

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/ngicks/go-fsys-helper/vroot"
	"golang.org/x/sync/errgroup"
)

// Log file names within the side-band fsys.
const (
	// LogFileName is the append-only journal file MetadataStoreLog writes.
	LogFileName = "vroot-overlayfs.log"
	// logTmpName is the scratch file used during compaction; it is renamed
	// over LogFileName atomically.
	logTmpName = "vroot-overlayfs.log.tmp"
	// logHeader is written as the first line of a freshly compacted file. It is
	// ignored on Load.
	logHeader = "# vroot-overlayfs log v1"
)

// SyncMode selects how [MetadataStoreLog] makes journal appends durable.
//
// The zero value ([SyncDefault]) resolves to [SyncBatched], the default.
type SyncMode int

const (
	// SyncDefault is the zero value and resolves to SyncBatched.
	SyncDefault SyncMode = iota
	// SyncPerOp appends and fsyncs synchronously on every change. Strongest
	// durability, highest per-op cost.
	SyncPerOp
	// SyncBatched (the default) updates the in-memory set immediately and hands
	// the record to a background writer that batches appends and fsyncs once
	// per batch. Keeps fsync off the calling op; a crash without a Flush loses
	// only the last unflushed window.
	SyncBatched
	// SyncNone appends but never fsyncs (until an explicit Flush). Fastest,
	// weakest durability.
	SyncNone
)

func (m SyncMode) resolve() SyncMode {
	if m == SyncDefault {
		return SyncBatched
	}
	return m
}

// LogOption configures [NewMetadataStoreLog].
type LogOption struct {
	// CompactFactor triggers compaction when appended lines exceed
	// live*CompactFactor. Zero defaults to 2.
	CompactFactor int
	// CompactMin is the floor below which compaction never triggers. Zero
	// defaults to 64.
	CompactMin int
	// Sync selects the durability mode. The zero value resolves to SyncBatched.
	// Flush forces durability regardless of mode.
	Sync SyncMode
}

func (o *LogOption) factor() int {
	if o == nil || o.CompactFactor <= 0 {
		return 2
	}
	return o.CompactFactor
}

func (o *LogOption) min() int {
	if o == nil || o.CompactMin <= 0 {
		return 64
	}
	return o.CompactMin
}

func (o *LogOption) sync() SyncMode {
	if o == nil {
		return SyncDefault.resolve()
	}
	return o.Sync.resolve()
}

// record is a single pending journal change handed to the background writer.
type record struct {
	set    bool // true: set, false: clear
	opaque bool // true: opaque, false: whiteout
	name   string
	done   chan struct{} // closed once durably written (used by Flush/PerOp)
}

func (r record) encode() string {
	var b strings.Builder
	switch {
	case r.set && !r.opaque:
		b.WriteString("+w ")
	case !r.set && !r.opaque:
		b.WriteString("-w ")
	case r.set && r.opaque:
		b.WriteString("+o ")
	default:
		b.WriteString("-o ")
	}
	b.WriteString(strconv.Quote(r.name))
	b.WriteByte('\n')
	return b.String()
}

var _ MetadataStore = (*MetadataStoreLog)(nil)

// MetadataStoreLog is the default durable [MetadataStore]: an append-only
// journal file with threshold compaction and group commit.
//
// The journal lives in the provided side-band fsys (the caller chooses it).
// Each change is one newline-terminated line whose path is strconv-quoted:
//
//	+w "dir/removed.txt"   set whiteout
//	-w "dir/removed.txt"   clear whiteout
//	+o "recreated-dir"     set opaque
//	-o "recreated-dir"     clear opaque
//
// Load replays the file last-writer-wins, tolerating a trailing partial line
// (a crash mid-append) and ignoring unknown record types. When appended lines
// exceed max(live*CompactFactor, CompactMin) the live set is rewritten to a
// tmp file, fsynced, and renamed over the log (amortizing O(N) to O(1)/change);
// at compaction, records subsumed by an ancestor whiteout are dropped.
//
// Durability follows [SyncMode]; Flush forces pending records durable
// regardless of mode. Close flushes, stops the background writer, and closes
// the held-open append handle. Close is not part of the [MetadataStore]
// interface.
type MetadataStoreLog struct {
	fsys   vroot.Fs[vroot.File]
	mode   SyncMode
	factor int
	minTh  int

	// mu guards the live sets, the append handle, appended/compaction
	// accounting, and the closed flag. It is NOT held across fsync in Batched
	// mode (the background writer takes it only briefly per append).
	mu        sync.Mutex
	whiteouts map[string]struct{}
	opaques   map[string]struct{}
	appended  int // lines appended since last compaction
	appender  vroot.File
	closed    bool

	// Batched-mode background writer.
	group  *errgroup.Group
	cancel context.CancelFunc
	ch     chan record
}

// NewMetadataStoreLog opens (or creates) the journal in fsys and replays it
// into memory. opt nil uses defaults (CompactFactor 2, CompactMin 64,
// Sync Batched). The returned store holds an O_APPEND handle open for its
// lifetime; call Close to release it.
func NewMetadataStoreLog(fsys vroot.Fs[vroot.File], opt *LogOption) *MetadataStoreLog {
	m := &MetadataStoreLog{
		fsys:      fsys,
		mode:      opt.sync(),
		factor:    opt.factor(),
		minTh:     opt.min(),
		whiteouts: make(map[string]struct{}),
		opaques:   make(map[string]struct{}),
	}
	if m.mode == SyncBatched {
		ctx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel
		m.ch = make(chan record, 256)
		g, _ := errgroup.WithContext(ctx)
		m.group = g
		g.Go(func() error {
			m.writeLoop(ctx)
			return nil
		})
	}
	return m
}

// Load replays the journal into the live sets and returns them. It is safe to
// call before any append; the parse tolerates a missing file, a trailing
// partial line, and unknown record types.
func (m *MetadataStoreLog) Load() (whiteouts []string, opaques []string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.appender == nil {
		if err := m.replayLocked(); err != nil {
			return nil, nil, err
		}
		if err := m.openAppenderLocked(); err != nil {
			return nil, nil, err
		}
	}
	whiteouts = setToSlice(m.whiteouts)
	opaques = setToSlice(m.opaques)
	return whiteouts, opaques, nil
}

func setToSlice(s map[string]struct{}) []string {
	out := make([]string, 0, len(s))
	for p := range s {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// replayLocked reads the whole log file and replays it into the live sets.
func (m *MetadataStoreLog) replayLocked() error {
	f, err := m.fsys.Open(LogFileName)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	m.replayBytes(data)
	return nil
}

// replayBytes parses raw journal bytes and applies them last-writer-wins.
// A trailing partial line (no newline at EOF) is ignored.
func (m *MetadataStoreLog) replayBytes(data []byte) {
	for len(data) > 0 {
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			// Trailing partial line: ignore it (crash mid-append).
			break
		}
		m.applyLine(string(data[:idx]))
		data = data[idx+1:]
	}
}

func (m *MetadataStoreLog) applyLine(line string) {
	line = strings.TrimRight(line, "\r")
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return
	}
	// Expect "<2-char op> <quoted path>".
	op, rest, ok := strings.Cut(trimmed, " ")
	if !ok {
		return
	}
	rest = strings.TrimSpace(rest)
	name, err := strconv.Unquote(rest)
	if err != nil {
		return
	}
	cleaned, err := validateName(name)
	if err != nil {
		return
	}
	switch op {
	case "+w":
		m.whiteouts[cleaned] = struct{}{}
	case "-w":
		delete(m.whiteouts, cleaned)
	case "+o":
		m.opaques[cleaned] = struct{}{}
	case "-o":
		delete(m.opaques, cleaned)
	default:
		// Unknown record type: ignore (forward-compat).
	}
}

func (m *MetadataStoreLog) openAppenderLocked() error {
	if m.appender != nil {
		return nil
	}
	f, err := m.fsys.OpenFile(LogFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	m.appender = f
	return nil
}

// ensureOpenLocked lazily replays + opens on first mutation if Load was never
// called.
func (m *MetadataStoreLog) ensureOpenLocked() error {
	if m.appender != nil {
		return nil
	}
	if err := m.replayLocked(); err != nil {
		return err
	}
	return m.openAppenderLocked()
}

func (m *MetadataStoreLog) SetWhiteout(name string) error   { return m.change(true, false, name) }
func (m *MetadataStoreLog) ClearWhiteout(name string) error { return m.change(false, false, name) }
func (m *MetadataStoreLog) SetOpaque(dir string) error      { return m.change(true, true, dir) }
func (m *MetadataStoreLog) ClearOpaque(dir string) error    { return m.change(false, true, dir) }

func (m *MetadataStoreLog) change(set, opaque bool, name string) error {
	cleaned, err := validateName(name)
	if err != nil {
		return err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return fs.ErrClosed
	}
	if err := m.ensureOpenLocked(); err != nil {
		m.mu.Unlock()
		return err
	}
	// Update the in-memory truth immediately so Load on this instance round-trips.
	target := m.whiteouts
	if opaque {
		target = m.opaques
	}
	if set {
		target[cleaned] = struct{}{}
	} else {
		delete(target, cleaned)
	}

	rec := record{set: set, opaque: opaque, name: cleaned}

	switch m.mode {
	case SyncBatched:
		ch := m.ch
		m.mu.Unlock()
		ch <- rec
		return nil
	default: // SyncPerOp, SyncNone
		err := m.appendLocked(rec)
		if err != nil {
			m.mu.Unlock()
			return err
		}
		if m.mode == SyncPerOp {
			err = m.appender.Sync()
		}
		var cerr error
		if m.shouldCompactLocked() {
			cerr = m.compactLocked()
		}
		m.mu.Unlock()
		if err != nil {
			return err
		}
		return cerr
	}
}

// appendLocked writes one record line through the held-open append handle.
// mu must be held.
func (m *MetadataStoreLog) appendLocked(rec record) error {
	if _, err := m.appender.WriteString(rec.encode()); err != nil {
		return err
	}
	m.appended++
	return nil
}

func (m *MetadataStoreLog) liveCountLocked() int {
	return len(m.whiteouts) + len(m.opaques)
}

func (m *MetadataStoreLog) shouldCompactLocked() bool {
	threshold := max(m.liveCountLocked()*m.factor, m.minTh)
	return m.appended > threshold
}

// writeLoop is the Batched-mode background writer. It drains records, batching
// appends and fsyncing once per drained batch.
func (m *MetadataStoreLog) writeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// Drain anything already queued, then exit.
			m.commitBatch(m.drainQueued(nil))
			return
		case rec, ok := <-m.ch:
			if !ok {
				return
			}
			m.commitBatch(m.drainQueued([]record{rec}))
		}
	}
}

// drainQueued non-blockingly appends every record currently buffered in the
// channel to batch and returns it.
func (m *MetadataStoreLog) drainQueued(batch []record) []record {
	for {
		select {
		case rec, ok := <-m.ch:
			if !ok {
				return batch
			}
			batch = append(batch, rec)
		default:
			return batch
		}
	}
}

// commitBatch appends every payload record in batch, fsyncs once, compacts if
// over threshold, and signals any Flush barriers (records with a done channel).
// Append/sync errors are dropped: in write-behind modes there is no caller to
// return them to, and a subsequent Flush in PerOp/None surfaces I/O failure.
func (m *MetadataStoreLog) commitBatch(batch []record) {
	m.mu.Lock()
	for _, rec := range batch {
		if rec.name == "" {
			// A Flush barrier carries no payload; just signal after sync below.
			continue
		}
		_ = m.appendLocked(rec)
	}
	if m.appender != nil {
		_ = m.appender.Sync()
		if m.shouldCompactLocked() {
			_ = m.compactLocked()
		}
	}
	m.mu.Unlock()

	for _, rec := range batch {
		if rec.done != nil {
			close(rec.done)
		}
	}
}

// Flush forces pending records durable regardless of mode.
func (m *MetadataStoreLog) Flush() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return fs.ErrClosed
	}
	mode := m.mode
	if m.appender == nil {
		// Nothing was ever opened/written.
		m.mu.Unlock()
		return nil
	}
	if mode != SyncBatched {
		err := m.appender.Sync()
		m.mu.Unlock()
		return err
	}
	ch := m.ch
	m.mu.Unlock()

	// Enqueue a barrier and wait for the writer to have flushed up to it.
	done := make(chan struct{})
	ch <- record{done: done}
	<-done
	return nil
}

// compactLocked rewrites the live set to logTmpName, fsyncs it, renames it over
// the log, and reopens the append handle. mu must be held. At compaction,
// records subsumed by an ancestor whiteout are dropped (decision 25).
func (m *MetadataStoreLog) compactLocked() error {
	whiteouts := setToSlice(m.whiteouts)
	opaques := setToSlice(m.opaques)

	// Drop any whiteout/opaque whose strict ancestor dir is itself whited out.
	whiteouts = dropSubsumed(whiteouts, m.whiteouts)
	opaques = dropSubsumed(opaques, m.whiteouts)

	_ = m.fsys.Remove(logTmpName)
	tmp, err := m.fsys.OpenFile(logTmpName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(tmp)
	writeErr := func() error {
		if _, err := w.WriteString(logHeader + "\n"); err != nil {
			return err
		}
		for _, p := range whiteouts {
			if _, err := w.WriteString("+w " + strconv.Quote(p) + "\n"); err != nil {
				return err
			}
		}
		for _, p := range opaques {
			if _, err := w.WriteString("+o " + strconv.Quote(p) + "\n"); err != nil {
				return err
			}
		}
		return w.Flush()
	}()
	if writeErr != nil {
		_ = tmp.Close()
		_ = m.fsys.Remove(logTmpName)
		return writeErr
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = m.fsys.Remove(logTmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = m.fsys.Remove(logTmpName)
		return err
	}

	// Close the current append handle before renaming over the log so backends
	// that dislike replacing an open file (Windows-like) stay correct.
	if m.appender != nil {
		if err := m.appender.Close(); err != nil {
			return err
		}
		m.appender = nil
	}
	if err := m.fsys.Rename(logTmpName, LogFileName); err != nil {
		// Try to recover an append handle so the store stays usable.
		_ = m.openAppenderLocked()
		return err
	}
	if err := m.openAppenderLocked(); err != nil {
		return err
	}
	m.appended = 0
	return nil
}

// dropSubsumed returns paths from sorted whose strict ancestor is in
// whiteouts (and thus masks them).
func dropSubsumed(sorted []string, whiteouts map[string]struct{}) []string {
	out := sorted[:0:0]
	for _, p := range sorted {
		if ancestorWhited(p, whiteouts) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// ancestorWhited reports whether some STRICT ancestor dir of p is in whiteouts.
func ancestorWhited(p string, whiteouts map[string]struct{}) bool {
	for {
		parent := path.Dir(p)
		if parent == "." || parent == p {
			return false
		}
		if _, ok := whiteouts[parent]; ok {
			return true
		}
		p = parent
	}
}

// Close flushes pending records, stops the background writer, and closes the
// held-open append handle. It is idempotent. Close is NOT part of the
// [MetadataStore] interface.
func (m *MetadataStoreLog) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	mode := m.mode
	m.mu.Unlock()

	if mode == SyncBatched {
		// Stop accepting and let the writer drain queued records + fsync.
		m.cancel()
		_ = m.group.Wait()
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	var err error
	if m.appender != nil {
		if mode != SyncBatched {
			err = m.appender.Sync()
		}
		if cerr := m.appender.Close(); cerr != nil && err == nil {
			err = cerr
		}
		m.appender = nil
	}
	return err
}
