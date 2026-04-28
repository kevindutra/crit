package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/google/uuid"

	"github.com/kevindutra/crit/internal/document"
	gitpkg "github.com/kevindutra/crit/internal/git"
	"github.com/kevindutra/crit/internal/review"
)

type pane int

const (
	contentPane pane = iota
	commentPane
)

type modalType int

const (
	noModal modalType = iota
	commentModal
	editModal
)

// gutterWidth is the total width of the left gutter: line number (5) + marker (1) + space (1).
const gutterWidth = 7

type AppModel struct {
	width, height int
	focused       pane
	modal         modalType

	// Multi-file tabs (code review mode)
	tabs         []FileTab
	activeTab    int
	multiFile    bool // true when in code review mode
	tabSearching bool
	tabSearch    string
	tabMatches   []int // indices of matching tabs during search

	// Single-file mode (legacy)
	filePath string

	detached bool

	contentViewport viewport.Model
	commentViewport viewport.Model
	modalTextarea   textarea.Model

	// Editing state
	editingID  string // ID of the comment being edited
	modalFocus int    // 0=textarea, 1=save button, 2=cancel button, 3=delete button (edit modal only)

	// Session-only: when false, resolved comments are filtered out of all
	// rendering and navigation. Resets to false on every TUI launch.
	showResolved bool

	err error
}

// visibleComments returns the slice of comments that should participate in
// rendering and navigation given the current showResolved toggle. Filtering
// happens here so every consumer (sidebar, annotations, scroll math, [/]
// navigation) sees the same view.
func (m *AppModel) visibleComments(t *FileTab) []review.Comment {
	if t.state == nil {
		return nil
	}
	if m.showResolved {
		return t.state.Comments
	}
	out := make([]review.Comment, 0, len(t.state.Comments))
	for _, c := range t.state.Comments {
		if !c.Resolved {
			out = append(out, c)
		}
	}
	return out
}

// commentCounts returns the unresolved (new) and resolved totals for a tab,
// regardless of the showResolved toggle.
func (m *AppModel) commentCounts(t *FileTab) (newCount, resolvedCount int) {
	if t.state == nil {
		return 0, 0
	}
	return t.state.Counts()
}

// formatCommentSummary renders the count fragment used in headers:
//   - "N new"               when no resolved comments exist
//   - "N new · M hidden"    when resolved exist but are filtered out
//   - "N new · M resolved"  when the user has toggled resolved visible
func (m *AppModel) formatCommentSummary(t *FileTab) string {
	newCount, resolvedCount := m.commentCounts(t)
	if resolvedCount == 0 {
		return fmt.Sprintf("%d new", newCount)
	}
	if m.showResolved {
		return fmt.Sprintf("%d new · %d resolved", newCount, resolvedCount)
	}
	return fmt.Sprintf("%d new · %d hidden", newCount, resolvedCount)
}

// tab returns the active FileTab. Panics if no tabs exist.
func (m *AppModel) tab() *FileTab {
	return &m.tabs[m.activeTab]
}

func NewApp(filePath string) AppModel {
	ta := textarea.New()
	ta.Placeholder = "Type your comment..."
	ta.ShowLineNumbers = false
	ta.CharLimit = 2000

	tab := FileTab{
		path:       filePath,
		cursorLine: 1,
	}

	return AppModel{
		filePath:        filePath,
		tabs:            []FileTab{tab},
		activeTab:       0,
		detached:        os.Getenv("CRIT_DETACHED") == "1",
		contentViewport: viewport.New(),
		commentViewport: viewport.New(),
		modalTextarea:   ta,
	}
}

// NewCodeReviewApp creates a multi-file code review TUI.
func NewCodeReviewApp(files []gitpkg.FileChange, ref string) AppModel {
	ta := textarea.New()
	ta.Placeholder = "Type your comment..."
	ta.ShowLineNumbers = false
	ta.CharLimit = 2000

	// Sort files alphabetically by path
	sortedFiles := make([]gitpkg.FileChange, len(files))
	copy(sortedFiles, files)
	sort.Slice(sortedFiles, func(i, j int) bool {
		return sortedFiles[i].Path < sortedFiles[j].Path
	})

	tabs := make([]FileTab, 0, len(sortedFiles))
	for _, f := range sortedFiles {
		var diff *gitpkg.DiffInfo
		if f.Status != gitpkg.StatusDeleted && f.Status != gitpkg.StatusBinary {
			diff, _ = gitpkg.DiffFile(f.Path, ref)
		}
		ft := newFileTab(f.Path, diff)
		if f.Status == gitpkg.StatusBinary {
			ft.isBinary = true
		}
		if f.Status == gitpkg.StatusDeleted {
			ft.isDeleted = true
		}
		tabs = append(tabs, ft)
	}

	return AppModel{
		tabs:            tabs,
		activeTab:       0,
		multiFile:       true,
		detached:        os.Getenv("CRIT_DETACHED") == "1",
		contentViewport: viewport.New(),
		commentViewport: viewport.New(),
		modalTextarea:   ta,
	}
}

func (m AppModel) Init() tea.Cmd {
	return tea.Batch(m.loadDocuments(), tea.RequestBackgroundColor)
}

func (m AppModel) loadDocuments() tea.Cmd {
	return func() tea.Msg {
		for _, tab := range m.tabs {
			if tab.isBinary || tab.isDeleted {
				continue
			}
			if _, err := document.Load(tab.path); err != nil {
				return errMsg{err}
			}
		}
		return docRenderedMsg{}
	}
}

// selectionRange returns the ordered start/end of the current selection.
// If not selecting, returns cursorLine, cursorLine.
func (m *AppModel) selectionRange() (int, int) {
	t := m.tab()
	if !t.selecting {
		return t.cursorLine, t.cursorLine
	}
	start, end := t.selectAnchor, t.cursorLine
	if start > end {
		start, end = end, start
	}
	return start, end
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		initAdaptiveStyles(msg.IsDark())
		if len(m.tabs) > 0 && m.tab().state != nil {
			m.rebuildContent()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalculateLayout()
		if len(m.tabs) > 0 && m.tab().state != nil {
			m.rebuildContent()
		}
		return m, nil

	case docRenderedMsg:
		// Pre-existing comments load as resolved so they're hidden by default
		// and excluded from `crit status`; persisting the migration keeps
		// disk and in-memory state in sync.
		for i := range m.tabs {
			t := &m.tabs[i]
			if t.isBinary || t.isDeleted {
				t.state = &review.ReviewState{
					File:     t.path,
					Comments: []review.Comment{},
				}
				continue
			}
			doc, _ := document.Load(t.path)
			t.doc = doc
			state, err := review.Load(t.path)
			if err != nil {
				state = &review.ReviewState{
					File:     t.path,
					Comments: []review.Comment{},
				}
			}
			t.state = state
			if state.MarkAllResolved() {
				if err := review.Save(state); err != nil {
					m.err = fmt.Errorf("migrating resolved comments for %s: %w", t.path, err)
					return m, nil
				}
			}
			t.ensureHighlightCache()
		}

		m.rebuildContent()
		m.updateCommentSidebar()
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
	}

	var cmd tea.Cmd
	if m.modal != noModal {
		m.modalTextarea, cmd = m.modalTextarea.Update(msg)
		return m, cmd
	}

	switch m.focused {
	case contentPane:
		m.contentViewport, cmd = m.contentViewport.Update(msg)
	case commentPane:
		m.commentViewport, cmd = m.commentViewport.Update(msg)
	}

	return m, cmd
}

func (m *AppModel) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.modal == commentModal || m.modal == editModal {
		return m.handleTextModal(msg)
	}

	// Tab search input mode
	if m.tabSearching {
		return m.handleTabSearch(msg)
	}

	t := m.tab()

	switch {
	case key.Matches(msg, keys.Quit):
		// Auto-save all tabs on quit
		for i := range m.tabs {
			if m.tabs[i].state != nil {
				review.Save(m.tabs[i].state)
			}
		}
		return m, tea.Quit

	case key.Matches(msg, keys.Cancel):
		// Esc cancels selection
		if t.selecting {
			t.selecting = false
			m.rebuildContent()
			return m, nil
		}
		return m, nil

	case key.Matches(msg, keys.Tab):
		if !t.selecting {
			if m.focused == contentPane {
				m.focused = commentPane
			} else {
				m.focused = contentPane
			}
			m.updateCommentSidebar()
			m.rebuildContent()
		}
		return m, nil

	case key.Matches(msg, keys.VisualMode):
		if m.focused == contentPane && t.doc != nil {
			if t.selecting {
				t.selecting = false
			} else {
				t.selecting = true
				t.selectAnchor = t.cursorLine
			}
			m.rebuildContent()
			return m, nil
		}

	case key.Matches(msg, keys.ToggleResolved):
		// Capture focused IDs against the OLD filter, flip, then restore by ID
		// so a same-line resolved annotation appearing/disappearing doesn't
		// silently shift focus to a different comment that landed at the same
		// index.
		var focusedAnnoID, focusedSidebarID string
		if t.cursorOnAnnotation {
			anns := m.annotationsAfterLine(t.cursorLine)
			if t.cursorAnnoIdx < len(anns) {
				focusedAnnoID = anns[t.cursorAnnoIdx].id
			}
		}
		if t.sidebarCursor >= 0 && t.sidebarCursor < len(t.sidebarItems) {
			focusedSidebarID = t.sidebarItems[t.sidebarCursor].id
		}

		m.showResolved = !m.showResolved
		m.updateCommentSidebar()

		if focusedAnnoID != "" {
			anns := m.annotationsAfterLine(t.cursorLine)
			idx := slices.IndexFunc(anns, func(a annotation) bool { return a.id == focusedAnnoID })
			if idx >= 0 {
				t.cursorOnAnnotation = true
				t.cursorAnnoIdx = idx
			} else {
				t.cursorOnAnnotation = false
				t.cursorAnnoIdx = 0
			}
		}
		if focusedSidebarID != "" {
			idx := slices.IndexFunc(t.sidebarItems, func(s sidebarItem) bool { return s.id == focusedSidebarID })
			if idx >= 0 {
				t.sidebarCursor = idx
			} else {
				t.sidebarCursor = 0
			}
		}

		m.rebuildContent()
		m.scrollToCursor()
		return m, nil
	}

	// Tab switching (multi-file mode)
	if m.multiFile && m.focused == contentPane && !t.selecting {
		switch {
		case key.Matches(msg, keys.PrevTab):
			if m.activeTab > 0 {
				m.activeTab--
				m.rebuildContent()
				m.updateCommentSidebar()
			}
			return m, nil
		case key.Matches(msg, keys.NextTab):
			if m.activeTab < len(m.tabs)-1 {
				m.activeTab++
				m.rebuildContent()
				m.updateCommentSidebar()
			}
			return m, nil
		case key.Matches(msg, keys.TabSearch):
			m.tabSearching = true
			m.tabSearch = ""
			m.tabMatches = nil
			return m, nil
		}
		// Number keys 1-9 for direct tab access
		if s := msg.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			idx := int(s[0]-'0') - 1
			if idx < len(m.tabs) {
				m.activeTab = idx
				m.rebuildContent()
				m.updateCommentSidebar()
			}
			return m, nil
		}
	}

	// Content pane cursor movement (annotation-aware)
	if m.focused == contentPane && t.doc != nil {
		moved := false
		switch {
		case key.Matches(msg, keys.Down):
			if t.cursorOnAnnotation {
				anns := m.annotationsAfterLine(t.cursorLine)
				if t.cursorAnnoIdx < len(anns)-1 {
					t.cursorAnnoIdx++
				} else {
					t.cursorOnAnnotation = false
					t.cursorAnnoIdx = 0
					if t.cursorLine < t.doc.LineCount() {
						t.cursorLine++
					}
				}
			} else {
				anns := m.annotationsAfterLine(t.cursorLine)
				if len(anns) > 0 {
					t.cursorOnAnnotation = true
					t.cursorAnnoIdx = 0
				} else if t.cursorLine < t.doc.LineCount() {
					t.cursorLine++
				}
			}
			moved = true
		case key.Matches(msg, keys.Up):
			if t.cursorOnAnnotation {
				if t.cursorAnnoIdx > 0 {
					t.cursorAnnoIdx--
				} else {
					t.cursorOnAnnotation = false
					t.cursorAnnoIdx = 0
				}
			} else {
				prevLine := t.cursorLine - 1
				if prevLine >= 1 {
					anns := m.annotationsAfterLine(prevLine)
					if len(anns) > 0 {
						t.cursorLine = prevLine
						t.cursorOnAnnotation = true
						t.cursorAnnoIdx = len(anns) - 1
					} else {
						t.cursorLine = prevLine
					}
				}
			}
			moved = true
		case key.Matches(msg, keys.HalfPageDown):
			t.cursorOnAnnotation = false
			t.cursorAnnoIdx = 0
			jump := m.contentViewport.Height() / 2
			t.cursorLine += jump
			if t.cursorLine > t.doc.LineCount() {
				t.cursorLine = t.doc.LineCount()
			}
			moved = true
		case key.Matches(msg, keys.HalfPageUp):
			t.cursorOnAnnotation = false
			t.cursorAnnoIdx = 0
			jump := m.contentViewport.Height() / 2
			t.cursorLine -= jump
			if t.cursorLine < 1 {
				t.cursorLine = 1
			}
			moved = true
		case key.Matches(msg, keys.Top):
			t.cursorOnAnnotation = false
			t.cursorAnnoIdx = 0
			t.cursorLine = 1
			moved = true
		case key.Matches(msg, keys.Bottom):
			t.cursorOnAnnotation = false
			t.cursorAnnoIdx = 0
			t.cursorLine = t.doc.LineCount()
			moved = true
		case key.Matches(msg, keys.NextComment):
			visible := m.visibleComments(t)
			if len(visible) > 0 {
				type target struct {
					endLine int
					idx     int
				}
				var best *target
				for _, c := range visible {
					endAt := c.EndAt()
					if endAt > t.cursorLine || (endAt == t.cursorLine && !t.cursorOnAnnotation) {
						if best == nil || endAt < best.endLine {
							best = &target{endLine: endAt, idx: 0}
						}
					}
				}
				if best == nil {
					for _, c := range visible {
						endAt := c.EndAt()
						if best == nil || endAt < best.endLine {
							best = &target{endLine: endAt, idx: 0}
						}
					}
				}
				if best != nil {
					t.cursorLine = best.endLine
					t.cursorOnAnnotation = true
					t.cursorAnnoIdx = best.idx
					moved = true
				}
			}
		case key.Matches(msg, keys.PrevComment):
			visible := m.visibleComments(t)
			if len(visible) > 0 {
				type target struct {
					endLine int
					idx     int
				}
				var best *target
				for _, c := range visible {
					endAt := c.EndAt()
					if endAt < t.cursorLine || (endAt == t.cursorLine && !t.cursorOnAnnotation) {
						if best == nil || endAt > best.endLine {
							best = &target{endLine: endAt, idx: 0}
						}
					}
				}
				if best == nil {
					for _, c := range visible {
						endAt := c.EndAt()
						if best == nil || endAt > best.endLine {
							best = &target{endLine: endAt, idx: 0}
						}
					}
				}
				if best != nil {
					t.cursorLine = best.endLine
					t.cursorOnAnnotation = true
					t.cursorAnnoIdx = best.idx
					moved = true
				}
			}
		case key.Matches(msg, keys.NextChange):
			if len(t.changeChunks) > 0 {
				target := -1
				for i, chunk := range t.changeChunks {
					if chunk.startLine > t.cursorLine {
						target = i
						break
					}
				}
				if target == -1 {
					target = 0
				}
				chunk := t.changeChunks[target]
				t.cursorLine = chunk.startLine
				t.cursorOnAnnotation = false
				t.cursorAnnoIdx = 0
				m.rebuildContent()
				m.scrollToChunk(chunk)
				return m, nil
			}
		case key.Matches(msg, keys.PrevChange):
			if len(t.changeChunks) > 0 {
				target := -1
				for i := len(t.changeChunks) - 1; i >= 0; i-- {
					if t.changeChunks[i].startLine < t.cursorLine {
						target = i
						break
					}
				}
				if target == -1 {
					target = len(t.changeChunks) - 1
				}
				chunk := t.changeChunks[target]
				t.cursorLine = chunk.startLine
				t.cursorOnAnnotation = false
				t.cursorAnnoIdx = 0
				m.rebuildContent()
				m.scrollToChunk(chunk)
				return m, nil
			}
		case key.Matches(msg, keys.Confirm):
			if t.cursorOnAnnotation {
				anns := m.annotationsAfterLine(t.cursorLine)
				if t.cursorAnnoIdx < len(anns) {
					ann := anns[t.cursorAnnoIdx]
					if ann.resolved {
						return m, nil
					}
					m.editingID = ann.id
					m.modal = editModal
					m.modalFocus = 0
					m.modalTextarea.Reset()
					m.modalTextarea.SetValue(ann.body)
					m.modalTextarea.Placeholder = "Edit comment..."
					m.modalTextarea.Focus()
					return m, nil
				}
			} else if t.state != nil {
				m.modal = commentModal
				m.modalFocus = 0
				m.modalTextarea.Placeholder = "Type your comment..."
				m.modalTextarea.Reset()
				m.modalTextarea.Focus()
				return m, nil
			}
		}

		if moved {
			m.rebuildContent()
			m.scrollToCursor()
			return m, nil
		}
	}

	// Comment pane navigation
	if m.focused == commentPane && len(t.sidebarItems) > 0 {
		sidebarMoved := false
		switch {
		case key.Matches(msg, keys.Down):
			if t.sidebarCursor < len(t.sidebarItems)-1 {
				t.sidebarCursor++
				sidebarMoved = true
			}
		case key.Matches(msg, keys.Up):
			if t.sidebarCursor > 0 {
				t.sidebarCursor--
				sidebarMoved = true
			}
		case key.Matches(msg, keys.Top):
			t.sidebarCursor = 0
			sidebarMoved = true
		case key.Matches(msg, keys.Bottom):
			t.sidebarCursor = len(t.sidebarItems) - 1
			sidebarMoved = true
		}
		if sidebarMoved {
			m.updateCommentSidebar()
			m.rebuildContent()
			sel := t.sidebarItems[t.sidebarCursor]
			t.cursorLine = sel.line
			m.scrollToAnnotation(sel.line, sel.endLine)
			return m, nil
		}

		// Enter to edit selected annotation
		if key.Matches(msg, keys.Confirm) {
			sel := t.sidebarItems[t.sidebarCursor]
			if sel.resolved {
				return m, nil
			}
			m.editingID = sel.id
			m.modal = editModal
			m.modalFocus = 0
			m.modalTextarea.Reset()
			m.modalTextarea.SetValue(sel.body)
			m.modalTextarea.Placeholder = "Edit comment..."
			m.modalTextarea.Focus()
			return m, nil
		}
	}

	return m, nil
}

func (m *AppModel) modalSubmit() {
	t := m.tab()
	body := strings.TrimSpace(m.modalTextarea.Value())
	if body == "" || t.state == nil {
		return
	}

	if m.modal == editModal {
		for i := range t.state.Comments {
			if t.state.Comments[i].ID == m.editingID {
				t.state.Comments[i].Body = body
				break
			}
		}
		m.editingID = ""
	} else {
		startLine, endLine := m.selectionRange()
		snippet := ""
		if t.doc != nil {
			snippet = strings.TrimSpace(t.doc.LineAt(startLine))
		}

		c := review.Comment{
			ID:             uuid.NewString()[:8],
			Line:           startLine,
			ContentSnippet: snippet,
			Body:           body,
			CreatedAt:      time.Now(),
		}
		if startLine != endLine {
			c.EndLine = endLine
		}
		t.state.AddComment(c)
	}

	review.Save(t.state)
	m.modal = noModal
	m.modalTextarea.Blur()
	t.selecting = false
	m.rebuildContent()
	m.updateCommentSidebar()
}

func (m *AppModel) modalDelete() {
	t := m.tab()
	if t.state == nil || m.editingID == "" {
		return
	}
	t.state.DeleteComment(m.editingID)
	m.editingID = ""
	review.Save(t.state)
	m.modal = noModal
	m.modalTextarea.Blur()
	t.cursorOnAnnotation = false
	t.cursorAnnoIdx = 0
	m.rebuildContent()
	m.updateCommentSidebar()
}

func (m *AppModel) handleTextModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Number of focusable elements: edit modal has 4 (textarea, save, cancel, delete), comment modal has 3
	focusCount := 3
	if m.modal == editModal {
		focusCount = 4
	}

	switch msg.String() {
	case "esc":
		m.modal = noModal
		m.modalTextarea.Blur()
		return m, nil
	case "tab", "shift+tab":
		if msg.String() == "shift+tab" {
			m.modalFocus = (m.modalFocus + focusCount - 1) % focusCount
		} else {
			m.modalFocus = (m.modalFocus + 1) % focusCount
		}
		if m.modalFocus == 0 {
			m.modalTextarea.Focus()
		} else {
			m.modalTextarea.Blur()
		}
		return m, nil
	case "enter":
		if m.modalFocus == 1 {
			m.modalSubmit()
			return m, nil
		} else if m.modalFocus == 2 {
			m.modal = noModal
			m.modalTextarea.Blur()
			return m, nil
		} else if m.modalFocus == 3 && m.modal == editModal {
			m.modalDelete()
			return m, nil
		}
	case "ctrl+s":
		m.modalSubmit()
		return m, nil
	}

	if m.modalFocus == 0 {
		var cmd tea.Cmd
		m.modalTextarea, cmd = m.modalTextarea.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *AppModel) handleTabSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.tabSearching = false
		m.tabSearch = ""
		m.tabMatches = nil
		return m, nil
	case "enter":
		if len(m.tabMatches) > 0 {
			m.activeTab = m.tabMatches[0]
			m.rebuildContent()
			m.updateCommentSidebar()
		}
		m.tabSearching = false
		m.tabSearch = ""
		m.tabMatches = nil
		return m, nil
	case "backspace":
		if len(m.tabSearch) > 0 {
			m.tabSearch = m.tabSearch[:len(m.tabSearch)-1]
			m.updateTabSearchMatches()
		}
		return m, nil
	case "tab":
		// Cycle to next match
		if len(m.tabMatches) > 1 {
			// Rotate matches
			m.tabMatches = append(m.tabMatches[1:], m.tabMatches[0])
		}
		return m, nil
	default:
		s := msg.String()
		if len(s) == 1 && s[0] >= ' ' && s[0] <= '~' {
			m.tabSearch += s
			m.updateTabSearchMatches()
		}
		return m, nil
	}
}

func (m *AppModel) updateTabSearchMatches() {
	m.tabMatches = nil
	if m.tabSearch == "" {
		return
	}
	query := strings.ToLower(m.tabSearch)
	for i, t := range m.tabs {
		if strings.Contains(strings.ToLower(t.path), query) {
			m.tabMatches = append(m.tabMatches, i)
		}
	}
}

func (m *AppModel) recalculateLayout() {
	headerHeight := 1
	if m.detached {
		headerHeight = 2
	}
	tabBarHeight := 0
	if m.multiFile {
		tabBarHeight = 3 // bordered tabs: top border + content + bottom border
	}
	footerHeight := 1
	tmuxPadding := 0
	if os.Getenv("TMUX") != "" {
		tmuxPadding = 1
	}
	frameBorderHeight := 0
	frameBorderWidth := 0
	if m.multiFile {
		frameBorderHeight = 1 // bottom border
		frameBorderWidth = 2  // left + right borders
	}
	mainHeight := m.height - headerHeight - tabBarHeight - footerHeight - frameBorderHeight - tmuxPadding

	commentWidth := m.width / 4
	if commentWidth < 20 {
		commentWidth = 20
	}
	contentWidth := m.width - commentWidth - frameBorderWidth

	m.contentViewport.SetWidth(contentWidth)
	m.contentViewport.SetHeight(mainHeight)
	m.commentViewport.SetWidth(commentWidth - 3) // -3 for left border + padding + margin
	m.commentViewport.SetHeight(mainHeight - 1)  // -1 for the "Comments (N)" header line

	modalWidth := m.width * 2 / 3
	if modalWidth < 50 {
		modalWidth = 50
	}
	if modalWidth > m.width-4 {
		modalWidth = m.width - 4
	}
	m.modalTextarea.SetWidth(modalWidth - 10)
	m.modalTextarea.SetHeight(6)
}

// annotationsAfterLine returns annotations that render after the given line
// (keyed by their endLine).
func (m *AppModel) annotationsAfterLine(lineNum int) []annotation {
	t := m.tab()
	var anns []annotation
	for _, c := range m.visibleComments(t) {
		if c.EndAt() == lineNum {
			anns = append(anns, annotation{
				id: c.ID, body: c.Body,
				line: c.Line, endLine: c.EndLine,
				resolved: c.Resolved,
			})
		}
	}
	return anns
}

// sidebarItem represents a comment in the sidebar list.
type sidebarItem struct {
	id       string
	line     int
	endLine  int
	body     string
	resolved bool
}

// annotation represents an inline comment to render.
type annotation struct {
	id       string
	body     string
	line     int
	endLine  int
	resolved bool
}

// rebuildContent renders the document line-by-line with cursor, selection,
// line numbers, and bordered inline annotations.
func (m *AppModel) rebuildContent() {
	t := m.tab()

	// Handle placeholder tabs
	if t.isBinary {
		m.contentViewport.SetContent("\n  Binary file changed — cannot display content.\n")
		return
	}
	if t.isDeleted {
		m.contentViewport.SetContent("\n  File deleted.\n")
		return
	}

	if t.doc == nil {
		return
	}

	visible := m.visibleComments(t)

	// Collect annotations keyed by the line they appear AFTER
	annosByEndLine := make(map[int][]annotation)
	for _, c := range visible {
		annosByEndLine[c.EndAt()] = append(annosByEndLine[c.EndAt()], annotation{
			id: c.ID, body: c.Body,
			line: c.Line, endLine: c.EndLine,
			resolved: c.Resolved,
		})
	}

	// Count how many comments cover each line (for overlap detection)
	annotatedLines := make(map[int]int)
	for _, c := range visible {
		for l := c.Line; l <= c.EndAt(); l++ {
			annotatedLines[l]++
		}
	}

	selStart, selEnd := m.selectionRange()

	// Determine which lines to highlight from selected annotation
	var sidebarHighlightStart, sidebarHighlightEnd int
	if m.focused == commentPane && len(t.sidebarItems) > 0 && t.sidebarCursor < len(t.sidebarItems) {
		sel := t.sidebarItems[t.sidebarCursor]
		sidebarHighlightStart = sel.line
		sidebarHighlightEnd = sel.line
		if sel.endLine > 0 {
			sidebarHighlightEnd = sel.endLine
		}
	} else if m.focused == contentPane && t.cursorOnAnnotation {
		anns := m.annotationsAfterLine(t.cursorLine)
		if t.cursorAnnoIdx < len(anns) {
			ann := anns[t.cursorAnnoIdx]
			sidebarHighlightStart = ann.line
			sidebarHighlightEnd = ann.line
			if ann.endLine > 0 {
				sidebarHighlightEnd = ann.endLine
			}
		}
	}

	contentWidth := m.contentViewport.Width()
	boxWidth := contentWidth - 7
	if boxWidth < 20 {
		boxWidth = 20
	}

	textWidth := contentWidth - 8
	if textWidth < 10 {
		textWidth = 10
	}

	// Use cached syntax highlighting
	isMarkdown := t.isMarkdown
	chromaLines := t.chromaLines

	// Detect table blocks so we can align columns across rows
	tableBlocks := detectTableBlocks(t.doc.Lines)
	tableBlockMap := make(map[int]*tableBlock)
	for i := range tableBlocks {
		tb := &tableBlocks[i]
		for l := tb.startLine; l <= tb.endLine; l++ {
			tableBlockMap[l] = tb
		}
	}

	// inlineBg applies a background color to just the content text (no full-width padding).
	// For content with embedded ANSI codes (Chroma), it re-injects the bg after resets.
	inlineBg := func(style lipgloss.Style, content string) string {
		bgAnsi := bgToAnsi(style.GetBackground())
		if bgAnsi == "" {
			return style.Render(content)
		}
		patched := strings.ReplaceAll(content, "\033[0m", "\033[0m"+bgAnsi)
		return bgAnsi + patched + "\033[0m"
	}

	var b strings.Builder
	b.Grow(len(t.doc.Lines) * 200) // pre-allocate to reduce allocations
	for i, line := range t.doc.Lines {
		lineNum := i + 1

		// Render deleted lines that appear before this line
		if dels, ok := t.deletedAfter[lineNum-1]; ok {
			cachedHL := t.deletedLineCache[lineNum-1]
			for di, del := range dels {
				delMarker := diffDeletedGutter.Render("-")
				delNum := diffDeletedLineNum.Render(fmt.Sprintf("%d", del.OldLineNum))
				delContent := del.Content
				if cachedHL != nil && di < len(cachedHL) {
					delContent = cachedHL[di]
				}
				b.WriteString(fmt.Sprintf("%s%s %s\n", delMarker, delNum, inlineBg(diffDeletedLineBg, delContent)))
			}
		}

		isCursor := lineNum == t.cursorLine
		isSelected := t.selecting && lineNum >= selStart && lineNum <= selEnd
		isSidebarHighlight := sidebarHighlightStart > 0 && lineNum >= sidebarHighlightStart && lineNum <= sidebarHighlightEnd
		isChanged := t.changedLines != nil && t.changedLines[lineNum]

		// Marker column
		var marker string
		if isCursor && !t.cursorOnAnnotation {
			marker = cursorMarker.Render(">")
		} else if isSelected {
			marker = selectedMarker.Render("|")
		} else if isSidebarHighlight {
			marker = cursorMarker.Render(">")
		} else if count, ok := annotatedLines[lineNum]; ok && count > 0 {
			if count > 1 {
				marker = gutterOverlap.Render("◆")
			} else {
				marker = annotationGutter.Render("■")
			}
		} else if isChanged {
			marker = diffAddedGutter.Render("+")
		} else {
			marker = " "
		}

		// Line number
		var numStr string
		if isCursor {
			numStr = cursorLineNumStyle.Render(fmt.Sprintf("%d", lineNum))
		} else if isSelected {
			numStr = selectedLineNumStyle.Render(fmt.Sprintf("%d", lineNum))
		} else {
			numStr = lineNumStyle.Render(fmt.Sprintf("%d", lineNum))
		}

		// Check if this line is part of a table block
		if tb, inTable := tableBlockMap[lineNum]; inTable {
			var styledLine string
			if reTableSep.MatchString(line) {
				styledLine = formatTableSep(tb.colWidths)
			} else {
				isHeader := lineNum == tb.startLine
				styledLine = formatTableRow(line, tb.colWidths, isHeader)
			}

			if isSelected {
				styledLine = inlineBg(selectedLineBg, styledLine)
			} else if isSidebarHighlight {
				styledLine = inlineBg(sidebarHighlightBg, styledLine)
			} else if isChanged {
				styledLine = inlineBg(diffChangedLineBg, styledLine)
			}

			b.WriteString(fmt.Sprintf("%s%s %s\n", marker, numStr, styledLine))
		} else {
			// Get the display content: Chroma-highlighted or raw
			displayLine := line
			if !isMarkdown && chromaLines != nil && i < len(chromaLines) {
				displayLine = chromaLines[i]
			}

			// For Chroma-highlighted content, we skip wrapping (ANSI codes break lipgloss.Wrap)
			// and apply background overlays directly.
			if !isMarkdown && chromaLines != nil && i < len(chromaLines) {
				styledLine := displayLine
				if isSelected {
					styledLine = inlineBg(selectedLineBg, styledLine)
				} else if isSidebarHighlight {
					styledLine = inlineBg(sidebarHighlightBg, styledLine)
				} else if isChanged {
					styledLine = inlineBg(diffChangedLineBg, styledLine)
				}
				b.WriteString(fmt.Sprintf("%s%s %s\n", marker, numStr, styledLine))
			} else {
				// Markdown or plain text path with wrapping
				styleFunc := func(s string) string { return s }
				if isMarkdown {
					styleFunc = func(s string) string { return highlightMarkdown(s) }
				}
				if isSelected {
					styleFunc = func(s string) string { return inlineBg(selectedLineBg, s) }
				} else if isSidebarHighlight {
					styleFunc = func(s string) string { return inlineBg(sidebarHighlightBg, s) }
				} else if isChanged {
					base := styleFunc
					styleFunc = func(s string) string { return inlineBg(diffChangedLineBg, base(s)) }
				}

				wrapped := lipgloss.Wrap(line, textWidth, "")
				wrappedLines := strings.Split(wrapped, "\n")
				for wi, wl := range wrappedLines {
					if wi == 0 {
						b.WriteString(fmt.Sprintf("%s%s %s\n", marker, numStr, styleFunc(wl)))
					} else {
						b.WriteString(fmt.Sprintf(" %s %s\n", continuationGutter, styleFunc(wl)))
					}
				}
			}
		}

		// Render inline annotations after this line
		if anns, ok := annosByEndLine[lineNum]; ok {
			for idx, ann := range anns {
				focused := m.focused == contentPane && t.cursorOnAnnotation && t.cursorLine == lineNum && t.cursorAnnoIdx == idx
				b.WriteString(m.renderAnnotationBox(ann, boxWidth, focused))
			}
		}
	}

	m.contentViewport.SetContent(b.String())
}

// renderAnnotationBox renders a bordered annotation box indented under the gutter.
func (m *AppModel) renderAnnotationBox(ann annotation, maxWidth int, focused bool) string {
	var lineLabel string
	if ann.endLine > 0 {
		lineLabel = fmt.Sprintf("L%d-%d", ann.line, ann.endLine)
	} else {
		lineLabel = fmt.Sprintf("L%d", ann.line)
	}

	labelStyle := inlineLabelComment
	boxStyle := inlineCommentBox
	labelText := "comment"
	if ann.resolved {
		labelStyle = resolvedInlineLabelComment
		boxStyle = resolvedInlineCommentBox
		labelText = "resolved"
	}

	var boxContent strings.Builder
	label := labelStyle.Render(labelText)
	lineRef := commentLineStyle.Render(lineLabel)
	boxContent.WriteString(fmt.Sprintf("%s %s\n", label, lineRef))
	boxContent.WriteString(clampLines(ann.body, 3))

	if focused {
		boxStyle = boxStyle.BorderForeground(warning)
	}
	box := boxStyle.Width(maxWidth).Render(boxContent.String())

	var prefix string
	if focused {
		cursor := lipgloss.NewStyle().Width(2).Render(cursorMarker.Render(">"))
		prefix = cursor + strings.Repeat(" ", gutterWidth-2)
	} else {
		prefix = strings.Repeat(" ", gutterWidth)
	}

	var b strings.Builder
	for _, line := range strings.Split(box, "\n") {
		b.WriteString(prefix + line + "\n")
	}
	return b.String()
}

var (
	reBold       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic     = regexp.MustCompile(`(?:^|[^*])\*([^*]+?)\*(?:[^*]|$)`)
	reCode       = regexp.MustCompile("`([^`]+)`")
	reLink       = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	reListItem   = regexp.MustCompile(`^(\s*[-*+]\s)(.*)$`)
	reCheckbox   = regexp.MustCompile(`^(\s*[-*+]\s)\[([ xX])\]\s(.*)$`)
	reNumList    = regexp.MustCompile(`^(\s*\d+\.\s)(.*)$`)
	reBlockquote = regexp.MustCompile(`^(\s*>\s?)(.*)$`)
	reHr         = regexp.MustCompile(`^(\s*)([-*_]{3,})\s*$`)
	reTableRow   = regexp.MustCompile(`^\s*\|.*\|\s*$`)
	reTableSep   = regexp.MustCompile(`^\s*\|[\s:]*[-]+[\s:|-]*\|\s*$`)
)

// highlightMarkdown applies markdown syntax highlighting to a single line.
func highlightMarkdown(line string) string {
	trimmed := strings.TrimSpace(line)

	if reHr.MatchString(line) {
		return mdHrStyle.Render("─────────────────────────────────")
	}

	if strings.HasPrefix(trimmed, "#### ") {
		return mdH4Style.Render(line)
	}
	if strings.HasPrefix(trimmed, "### ") {
		return mdH3Style.Render(line)
	}
	if strings.HasPrefix(trimmed, "## ") {
		return mdH2Style.Render(line)
	}
	if strings.HasPrefix(trimmed, "# ") {
		return mdH1Style.Render(line)
	}

	if reTableSep.MatchString(line) {
		return mdTableSepStyle.Render(line)
	}
	if reTableRow.MatchString(line) {
		cells := strings.Split(line, "|")
		var parts []string
		for i, cell := range cells {
			if i == 0 || i == len(cells)-1 {
				parts = append(parts, cell)
			} else {
				parts = append(parts, highlightInline(cell))
			}
		}
		return strings.Join(parts, mdTablePipe.Render("|"))
	}

	if loc := reBlockquote.FindStringSubmatchIndex(line); loc != nil {
		rest := line[loc[4]:loc[5]]
		return mdBlockquoteBar.Render("▎") + " " + mdBlockquoteStyle.Render(rest)
	}

	if loc := reCheckbox.FindStringSubmatchIndex(line); loc != nil {
		indent := line[loc[2]:loc[3]]
		checked := line[loc[4]:loc[5]]
		rest := line[loc[6]:loc[7]]
		if checked == "x" || checked == "X" {
			return indent + mdCheckboxDone.Render("✓") + " " + mdCheckboxDoneText.Render(rest)
		}
		return indent + mdCheckboxOpen.Render("☐") + " " + highlightInline(rest)
	}

	if loc := reListItem.FindStringSubmatchIndex(line); loc != nil {
		indent := line[loc[2]:loc[3]]
		rest := line[loc[4]:loc[5]]
		return mdListMarkerStyle.Render(indent) + highlightInline(rest)
	}
	if loc := reNumList.FindStringSubmatchIndex(line); loc != nil {
		marker := line[loc[2]:loc[3]]
		rest := line[loc[4]:loc[5]]
		return mdListMarkerStyle.Render(marker) + highlightInline(rest)
	}

	return highlightInline(line)
}

// tableBlock represents a contiguous range of markdown table lines.
type tableBlock struct {
	startLine int
	endLine   int
	colWidths []int
}

func detectTableBlocks(lines []string) []tableBlock {
	var blocks []tableBlock
	inTable := false
	var current tableBlock

	for i, line := range lines {
		isTable := reTableRow.MatchString(line) || reTableSep.MatchString(line)
		if isTable {
			if !inTable {
				inTable = true
				current = tableBlock{startLine: i + 1}
			}
			current.endLine = i + 1

			if !reTableSep.MatchString(line) {
				cells := parseTableCells(line)
				for len(current.colWidths) < len(cells) {
					current.colWidths = append(current.colWidths, 0)
				}
				for ci, cell := range cells {
					if len(cell) > current.colWidths[ci] {
						current.colWidths[ci] = len(cell)
					}
				}
			}
		} else {
			if inTable {
				blocks = append(blocks, current)
				inTable = false
			}
		}
	}
	if inTable {
		blocks = append(blocks, current)
	}
	return blocks
}

func parseTableCells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

func formatTableRow(line string, colWidths []int, isHeader bool) string {
	cells := parseTableCells(line)
	pipe := mdTablePipe.Render("│")

	var parts []string
	for ci := 0; ci < len(colWidths); ci++ {
		w := colWidths[ci]
		cell := ""
		if ci < len(cells) {
			cell = cells[ci]
		}
		padded := lipgloss.NewStyle().Width(w).Render(cell)
		if isHeader {
			parts = append(parts, mdTableHeaderStyle.Render(" "+padded+" "))
		} else {
			parts = append(parts, mdTableCellStyle.Render(" "+padded+" "))
		}
	}

	return pipe + strings.Join(parts, pipe) + pipe
}

func formatTableSep(colWidths []int) string {
	pipe := mdTablePipe.Render("│")
	var parts []string
	for _, w := range colWidths {
		parts = append(parts, mdTableSepStyle.Render(strings.Repeat("─", w+2)))
	}
	return pipe + strings.Join(parts, mdTablePipe.Render("┼")) + pipe
}

func highlightInline(line string) string {
	line = reCode.ReplaceAllStringFunc(line, func(match string) string {
		inner := match[1 : len(match)-1]
		return mdCodeStyle.Render(" " + inner + " ")
	})

	line = reBold.ReplaceAllStringFunc(line, func(match string) string {
		inner := match[2 : len(match)-2]
		return mdBoldStyle.Render(inner)
	})

	line = reItalic.ReplaceAllStringFunc(line, func(match string) string {
		start := 0
		end := len(match)
		if match[0] != '*' {
			start = 1
		}
		if match[end-1] != '*' {
			end--
		}
		inner := match[start+1 : end-1]
		prefix := match[:start]
		suffix := match[end:]
		return prefix + mdItalicStyle.Render(inner) + suffix
	})

	line = reLink.ReplaceAllStringFunc(line, func(match string) string {
		idx := strings.Index(match, "](")
		if idx < 0 {
			return match
		}
		text := match[1:idx]
		return mdLinkStyle.Render(text)
	})

	return line
}

func (m *AppModel) scrollToCursor() {
	t := m.tab()
	if t.doc == nil {
		return
	}

	renderedLine := 0
	extraCounts := m.extraLinesPerDocLine()
	for i := 1; i < t.cursorLine; i++ {
		renderedLine++
		renderedLine += extraCounts[i]
	}

	cursorBottom := renderedLine + 1 + extraCounts[t.cursorLine]

	vpHeight := m.contentViewport.Height()
	currentTop := m.contentViewport.YOffset()

	if renderedLine < currentTop {
		m.contentViewport.SetYOffset(renderedLine)
	}
	if cursorBottom > currentTop+vpHeight {
		m.contentViewport.SetYOffset(cursorBottom - vpHeight)
	}
}

const chunkScrollPadding = 4

// scrollToChunk scrolls the viewport to show the entire change chunk
// plus padding lines above and below for context.
func (m *AppModel) scrollToChunk(chunk changeChunk) {
	t := m.tab()
	if t.doc == nil {
		return
	}

	extraCounts := m.extraLinesPerDocLine()

	// Compute rendered line for chunk start - padding
	startLine := chunk.startLine - chunkScrollPadding
	if startLine < 1 {
		startLine = 1
	}
	startRendered := 0
	for i := 1; i < startLine; i++ {
		startRendered++
		startRendered += extraCounts[i]
	}

	// Compute rendered line for chunk end + padding
	endLine := chunk.endLine + chunkScrollPadding
	if endLine > t.doc.LineCount() {
		endLine = t.doc.LineCount()
	}
	endRendered := 0
	for i := 1; i <= endLine; i++ {
		endRendered++
		endRendered += extraCounts[i]
	}

	vpHeight := m.contentViewport.Height()

	// If the whole chunk+padding fits, position start at top
	if endRendered-startRendered <= vpHeight {
		m.contentViewport.SetYOffset(startRendered)
	} else {
		// Chunk is taller than viewport — just put cursor near top with padding
		m.contentViewport.SetYOffset(startRendered)
	}
}

func (m *AppModel) scrollToAnnotation(startLine, endLine int) {
	t := m.tab()
	if t.doc == nil {
		return
	}
	if endLine == 0 {
		endLine = startLine
	}

	extraCounts := m.extraLinesPerDocLine()

	startRendered := 0
	for i := 1; i < startLine; i++ {
		startRendered++
		startRendered += extraCounts[i]
	}

	endRendered := 0
	for i := 1; i <= endLine; i++ {
		endRendered++
		endRendered += extraCounts[i]
	}

	vpHeight := m.contentViewport.Height()

	offset := endRendered - vpHeight
	if offset < 0 {
		offset = 0
	}
	if offset > startRendered {
		offset = startRendered
	}

	m.contentViewport.SetYOffset(offset)
}

func (m *AppModel) extraLinesPerDocLine() map[int]int {
	t := m.tab()
	counts := make(map[int]int)
	if t.doc == nil {
		return counts
	}

	contentWidth := m.contentViewport.Width()
	textWidth := contentWidth - 8
	if textWidth < 10 {
		textWidth = 10
	}

	for i, line := range t.doc.Lines {
		lineNum := i + 1
		wrapped := lipgloss.Wrap(line, textWidth, "")
		wrapCount := strings.Count(wrapped, "\n")
		if wrapCount > 0 {
			counts[lineNum] += wrapCount
		}
	}

	for _, c := range m.visibleComments(t) {
		bodyLines := strings.Count(c.Body, "\n") + 1
		counts[c.EndAt()] += bodyLines + 3
	}

	// Account for deleted lines rendered before each doc line
	if t.deletedAfter != nil {
		for afterLine, dels := range t.deletedAfter {
			targetLine := afterLine + 1
			if targetLine < 1 {
				targetLine = 1
			}
			counts[targetLine] += len(dels)
		}
	}

	return counts
}

func (m *AppModel) updateCommentSidebar() {
	t := m.tab()
	if t.state == nil {
		return
	}

	t.sidebarItems = nil
	for _, c := range m.visibleComments(t) {
		t.sidebarItems = append(t.sidebarItems, sidebarItem{
			id: c.ID, line: c.Line, endLine: c.EndLine,
			body:     c.Body,
			resolved: c.Resolved,
		})
	}
	sort.Slice(t.sidebarItems, func(i, j int) bool { return t.sidebarItems[i].line < t.sidebarItems[j].line })

	if t.sidebarCursor >= len(t.sidebarItems) {
		t.sidebarCursor = len(t.sidebarItems) - 1
	}
	if t.sidebarCursor < 0 {
		t.sidebarCursor = 0
	}

	var b strings.Builder

	if len(t.sidebarItems) == 0 {
		_, resolvedCount := m.commentCounts(t)
		if !m.showResolved && resolvedCount > 0 {
			b.WriteString(commentStyle.Render("No new comments.\n\nPress 'a' to show resolved."))
		} else {
			b.WriteString(commentStyle.Render("No comments yet.\n\nPress enter to comment.\n\nUse 'v' to select\nmultiple lines first."))
		}
		m.commentViewport.SetContent(b.String())
		return
	}

	for idx, it := range t.sidebarItems {
		isSelected := m.focused == commentPane && idx == t.sidebarCursor

		var lineInfo string
		if it.endLine > 0 {
			lineInfo = fmt.Sprintf("L%d-%d", it.line, it.endLine)
		} else {
			lineInfo = fmt.Sprintf("L%d", it.line)
		}
		lineInfo = commentLineStyle.Render(lineInfo)

		cursorCol := lipgloss.NewStyle().Width(2)
		prefix := cursorCol.Render("")
		if isSelected {
			prefix = cursorCol.Render(cursorMarker.Render(">"))
		}

		b.WriteString(fmt.Sprintf("%s%s\n", prefix, lineInfo))

		clamped := clampLines(it.body, 3)
		bodyLines := strings.Split(clamped, "\n")
		bodyStyle := commentStyle
		if it.resolved {
			bodyStyle = resolvedCommentStyle
		}
		for i, bl := range bodyLines {
			styled := bl
			if isSelected {
				styled = sidebarSelectedText.Render(bl)
			} else {
				styled = bodyStyle.Render(bl)
			}
			b.WriteString(" " + styled)
			if i < len(bodyLines)-1 {
				b.WriteString("\n")
			}
		}
		b.WriteString("\n\n")
	}

	m.commentViewport.SetContent(b.String())
}

func (m AppModel) View() tea.View {
	if m.err != nil {
		v := tea.NewView(fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err))
		v.AltScreen = true
		return v
	}

	if m.width == 0 || len(m.tabs) == 0 || m.tab().state == nil {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}

	t := m.tab()

	// Header
	commentSummary := m.formatCommentSummary(t)
	displayPath := t.path
	if m.filePath != "" {
		displayPath = m.filePath
	}
	var headerContent string
	if t.selecting {
		start, end := m.selectionRange()
		selLabel := visualModeIndicator.Render("VISUAL")
		headerContent = fmt.Sprintf(" Crit: %s  %s L%d-%d", displayPath, selLabel, start, end)
	} else if t.doc != nil {
		headerContent = fmt.Sprintf(" Crit: %s  %s  L%d/%d", displayPath, commentSummary, t.cursorLine, t.doc.LineCount())
	} else {
		headerContent = fmt.Sprintf(" Crit: %s  %s", displayPath, commentSummary)
	}
	var header string
	if m.detached {
		claudeBanner := claudeStatusBar.Width(m.width).Render(" Claude Code is paused — review the document, then press q to submit")
		header = claudeBanner + "\n" + headerStyle.Width(m.width).Render(headerContent)
	} else {
		header = headerStyle.Width(m.width).Render(headerContent)
	}

	// Tab bar (multi-file mode)
	var tabBar string
	if m.multiFile {
		tabBar = m.renderTabBar()
	}

	// Content pane
	commentWidth := m.width / 4
	if commentWidth < 20 {
		commentWidth = 20
	}
	contentWidth := m.width - commentWidth

	panelHeight := m.contentViewport.Height()

	contentBox := lipgloss.NewStyle().
		Width(contentWidth).
		Height(panelHeight).
		Render(m.contentViewport.View())

	// Comment sidebar (left border to separate from content)
	sidebarBorderColor := subtle
	if m.focused == commentPane {
		sidebarBorderColor = accent
	}
	sidebarBorder := lipgloss.Border{Left: "│"}
	commentHeader := lipgloss.NewStyle().Bold(true).Foreground(accent).Render(fmt.Sprintf("Comments (%s)", commentSummary))
	commentBox := lipgloss.NewStyle().
		Border(sidebarBorder, false, false, false, true).
		BorderForeground(sidebarBorderColor).
		Width(commentWidth - 2).
		Height(panelHeight).
		PaddingLeft(1).
		Render(commentHeader + "\n" + m.commentViewport.View())

	mainRow := lipgloss.JoinHorizontal(lipgloss.Top, contentBox, commentBox)

	// Wrap content in a frame: │ left/right borders, ╰───╯ bottom.
	// The tab bar serves as the top border.
	if m.multiFile {
		borderColor := lipgloss.NewStyle().Foreground(accent)
		lines := strings.Split(mainRow, "\n")
		var framed strings.Builder
		left := borderColor.Render("│")
		right := borderColor.Render("│")
		for _, line := range lines {
			framed.WriteString(left + line + right + "\n")
		}
		bottom := borderColor.Render("╰" + strings.Repeat("─", m.width-2) + "╯")
		framed.WriteString(bottom)
		mainRow = framed.String()
	}

	footer := m.renderFooter()

	var sections []string
	sections = append(sections, header)
	if tabBar != "" {
		sections = append(sections, tabBar)
	}
	sections = append(sections, mainRow, footer)

	full := lipgloss.JoinVertical(lipgloss.Left, sections...)

	if m.modal != noModal {
		full = m.renderWithModal(full)
	}

	v := tea.NewView(full)
	v.AltScreen = true
	return v
}

// renderTabBar renders the tab bar for multi-file mode.
func (m *AppModel) renderTabBar() string {
	if m.tabSearching {
		prompt := tabSearchPromptStyle.Render("/")
		query := m.tabSearch
		matchInfo := ""
		if m.tabSearch != "" {
			matchInfo = fmt.Sprintf(" (%d matches)", len(m.tabMatches))
		}
		return prompt + query + footerStyle.Render(matchInfo)
	}

	// Disambiguate filenames — use basename unless there are collisions
	basenames := make(map[string]int)
	for _, t := range m.tabs {
		base := filepath.Base(t.path)
		basenames[base]++
	}

	type tabLabel struct {
		text     string
		rendered string
		width    int
	}

	labels := make([]tabLabel, len(m.tabs))
	for i, t := range m.tabs {
		label := filepath.Base(t.path)
		if basenames[label] > 1 {
			label = t.path
		}
		if n := len(t.changedLines); n > 0 {
			label += " " + tabChangeCount.Render(fmt.Sprintf("(+%d)", n))
		}
		labels[i] = tabLabel{text: label}
	}
	// Render a single tab with correct border corners for its position.
	// isFirst adjusts the left corner to connect to the outer frame border.
	renderTab := func(i int, isFirst bool) string {
		var style lipgloss.Style
		isActive := i == m.activeTab
		if isActive {
			style = activeTabStyle
		} else {
			style = inactiveTabStyle
		}
		border, _, _, _, _ := style.GetBorder()
		if isFirst && isActive {
			border.BottomLeft = "│"
		} else if isFirst && !isActive {
			border.BottomLeft = "├"
		}
		style = style.Border(border)
		return style.Render(labels[i].text)
	}
	for i := range labels {
		rendered := renderTab(i, i == 0)
		labels[i].rendered = rendered
		labels[i].width = lipgloss.Width(rendered)
	}

	// addFiller extends the tab bottom border to the full width,
	// connecting to the outer frame's right border.
	addFiller := func(row string) string {
		rowW := lipgloss.Width(row)
		if rowW >= m.width {
			return row
		}
		// 3 lines matching tab height: empty top, empty middle, ───╮ bottom
		gap := m.width - rowW
		topFill := strings.Repeat(" ", gap)
		midFill := strings.Repeat(" ", gap)
		botFill := strings.Repeat("─", gap-1) + "╮"
		filler := lipgloss.NewStyle().Foreground(accent).Render(
			topFill + "\n" + midFill + "\n" + botFill,
		)
		return lipgloss.JoinHorizontal(lipgloss.Top, row, filler)
	}

	// Check if all tabs fit
	totalWidth := 0
	for _, l := range labels {
		totalWidth += l.width
	}

	if totalWidth <= m.width {
		var tabs []string
		for i := range labels {
			tabs = append(tabs, labels[i].rendered)
		}
		row := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
		return addFiller(row)
	}

	// Overflow: show a window of tabs centered on the active tab.
	// Indicators are styled as bordered tabs to align with the tab row.
	renderIndicator := func(text string, isFirst bool) string {
		style := inactiveTabStyle.Foreground(subtle)
		border, _, _, _, _ := style.GetBorder()
		if isFirst {
			border.BottomLeft = "├"
		}
		style = style.Border(border)
		return style.Render(text)
	}

	leftIndicator := ""
	rightIndicator := ""
	leftW := 0
	rightW := 0
	// Pre-render indicators to know their width for available space calc
	if m.activeTab > 0 {
		leftIndicator = renderIndicator(fmt.Sprintf("↤ %d more", m.activeTab), true)
		leftW = lipgloss.Width(leftIndicator)
	}
	if m.activeTab < len(labels)-1 {
		rightIndicator = renderIndicator(fmt.Sprintf("%d more ↦", len(labels)-m.activeTab-1), false)
		rightW = lipgloss.Width(rightIndicator)
	}

	availWidth := m.width - leftW - rightW

	// Find the window of tabs that fits
	start := m.activeTab
	end := m.activeTab + 1
	used := labels[m.activeTab].width

	// Expand window outward from active tab
	for {
		expanded := false
		if start > 0 && used+labels[start-1].width <= availWidth {
			start--
			used += labels[start].width
			expanded = true
		}
		if end < len(labels) && used+labels[end].width <= availWidth {
			used += labels[end].width
			end++
			expanded = true
		}
		if !expanded {
			break
		}
	}

	// Re-render indicators with actual counts now that we know the visible window
	var parts []string
	if start > 0 {
		ind := renderIndicator(fmt.Sprintf("↤ %d more", start), true)
		parts = append(parts, ind)
	}
	for i := start; i < end; i++ {
		parts = append(parts, renderTab(i, i == start && start == 0))
	}
	if end < len(labels) {
		ind := renderIndicator(fmt.Sprintf("%d more ↦", len(labels)-end), false)
		parts = append(parts, ind)
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	return addFiller(row)
}

func (m AppModel) renderFooter() string {
	t := m.tabs[m.activeTab]
	k := func(key, desc string) string {
		return footerKeyStyle.Render(key) + " " + footerStyle.Render(desc)
	}

	var items []string
	if t.selecting {
		items = []string{
			k("j/k", "extend"),
			k("enter", "comment selection"),
			k("esc", "cancel"),
			k("v", "toggle select"),
		}
	} else {
		items = []string{
			k("j/k", "move"),
			k("[/]", "prev/next comment"),
			k("shift+↑↓", "page"),
			k("s", "sidebar"),
			k("a", "toggle resolved"),
			k("v", "select"),
			k("enter", "comment"),
			k("q", "save & quit"),
		}
		if m.multiFile {
			items = append([]string{
				k("tab/S-tab", "next/prev tab"),
				k("n/N", "next/prev change"),
			}, items...)
		}
	}

	return footerStyle.Width(m.width).Render(strings.Join(items, "  "))
}

func (m AppModel) renderModalButton(label, hint string, focused bool) string {
	btn := modalBtnLabel.Render(label)
	h := modalBtnHint.Render(hint)
	content := btn + " " + h
	if focused {
		return modalBtnFocused.Render(content)
	}
	return modalBtnNormal.Render(content)
}

func (m AppModel) renderDeleteButton(label string, focused bool) string {
	if focused {
		return modalDeleteBtnFocused.Render(label)
	}
	return modalBtnNormal.Render(modalDeleteBtnLabel.Render(label))
}

func (m AppModel) renderContextPreview(start, end, maxWidth int) string {
	t := m.tabs[m.activeTab]
	if t.doc == nil {
		return ""
	}
	var lines []string
	maxLineText := maxWidth - 7
	if maxLineText < 10 {
		maxLineText = 10
	}
	for i := start; i <= end && i <= t.doc.LineCount(); i++ {
		lineText := t.doc.LineAt(i)
		wrapped := lipgloss.Wrap(lineText, maxLineText, "")
		num := lineNumStyle.Render(fmt.Sprintf("%d", i))
		wrapLines := strings.Split(wrapped, "\n")
		for wi, wl := range wrapLines {
			if wi == 0 {
				lines = append(lines, num+" "+wl)
			} else {
				lines = append(lines, lipgloss.NewStyle().Width(6).Render("")+wl)
			}
		}
	}
	if len(lines) > 8 {
		lines = append(lines[:7], footerStyle.Render(fmt.Sprintf("  ... +%d more lines", len(lines)-7)))
	}
	return strings.Join(lines, "\n")
}

func (m AppModel) renderWithModal(background string) string {
	var modalContent string
	modalWidth := m.width * 2 / 3
	if modalWidth < 50 {
		modalWidth = 50
	}
	if modalWidth > m.width-4 {
		modalWidth = m.width - 4
	}
	innerWidth := modalWidth - 6

	switch m.modal {
	case commentModal:
		start, end := m.selectionRange()
		var title string
		if start != end {
			title = modalTitleStyle.Render(fmt.Sprintf("Add Comment (lines %d-%d)", start, end))
		} else {
			title = modalTitleStyle.Render(fmt.Sprintf("Add Comment (line %d)", start))
		}
		contextBox := contextBoxStyle.
			Width(innerWidth - 2).
			Render(m.renderContextPreview(start, end, innerWidth-4))

		saveBtn := m.renderModalButton("Save", "ctrl+s", m.modalFocus == 1)
		cancelBtn := m.renderModalButton("Cancel", "esc", m.modalFocus == 2)
		buttons := lipgloss.JoinHorizontal(lipgloss.Center, saveBtn, "  ", cancelBtn)

		modalContent = modalStyle.Width(modalWidth).Render(
			title + "\n" + contextBox + "\n\n" + m.modalTextarea.View() + "\n\n" + buttons)

	case editModal:
		title := modalTitleStyle.Render("Edit Comment")
		var contextSection string
		for _, c := range m.tabs[m.activeTab].state.Comments {
			if c.ID == m.editingID {
				start := c.Line
				end := c.EndLine
				if end == 0 {
					end = start
				}
				contextSection = contextBoxStyle.
					Width(innerWidth - 2).
					Render(m.renderContextPreview(start, end, innerWidth-4))
				break
			}
		}
		saveBtn := m.renderModalButton("Save", "ctrl+s", m.modalFocus == 1)
		cancelBtn := m.renderModalButton("Cancel", "esc", m.modalFocus == 2)
		deleteBtn := m.renderDeleteButton("Delete", m.modalFocus == 3)
		buttons := lipgloss.JoinHorizontal(lipgloss.Center, saveBtn, "  ", cancelBtn, "  ", deleteBtn)

		content := title + "\n"
		if contextSection != "" {
			content += contextSection + "\n\n"
		}
		content += m.modalTextarea.View() + "\n\n" + buttons
		modalContent = modalStyle.Width(modalWidth).Render(content)
	}

	bgW := lipgloss.Width(background)
	bgH := lipgloss.Height(background)

	modalW := lipgloss.Width(modalContent)
	modalH := lipgloss.Height(modalContent)

	mx := (bgW - modalW) / 2
	my := (bgH - modalH) / 2
	if mx < 0 {
		mx = 0
	}
	if my < 0 {
		my = 0
	}

	background = dimRendered(background, bgW, bgH)

	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modalContent).X(mx).Y(my).Z(1)

	comp := lipgloss.NewCompositor(bgLayer, modalLayer)
	return comp.Render()
}

func dimRendered(s string, w, h int) string {
	canvas := lipgloss.NewCanvas(w, h)
	canvas.Compose(lipgloss.NewLayer(s))

	dim := lipgloss.Color("#555555")
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cell := canvas.CellAt(x, y)
			if cell != nil {
				cell.Style.Fg = dim
			}
		}
	}
	return canvas.Render()
}

// clampLines truncates text to maxLines and appends "…" if truncated.
func clampLines(text string, maxLines int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines {
		return text
	}
	return strings.Join(lines[:maxLines], "\n") + "…"
}
