package main

import (
    "archive/zip"
    "bytes"
    "database/sql"
    "fmt"
    "image"
    "image/color"
    "image/png"
	"io/fs"
    "math"
    "sort"
    "strconv"
    "strings"

    "fyne.io/fyne/v2"
    "fyne.io/fyne/v2/app"
    "fyne.io/fyne/v2/canvas"
    "fyne.io/fyne/v2/container"
    "fyne.io/fyne/v2/dialog"
    "fyne.io/fyne/v2/layout"
    "fyne.io/fyne/v2/theme"
    "fyne.io/fyne/v2/widget"
    
    fynetooltip "github.com/dweymouth/fyne-tooltip"

    _ "github.com/go-sql-driver/mysql"
    _ "embed"
)

//go:embed static/icons/icons.zip
var iconsZip []byte

var iconsFS fs.FS

var iconLookup map[string]string

func init() {
    r, err := zip.NewReader(bytes.NewReader(iconsZip), int64(len(iconsZip)))
    if err != nil {
        panic(err)
    }
    iconsFS = r

    iconLookup = make(map[string]string)
    fs.WalkDir(iconsFS, ".", func(path string, d fs.DirEntry, err error) error {
        if !d.IsDir() {
            // store lowercase → actual name
            iconLookup[strings.ToLower(path)] = path
        }
        return nil
    })
}

const (
    MAX_NUM_TALENT_TIERS = 15
    NUM_TALENT_COLUMNS   = 4
)

type TalentTab struct {
    ID             int
    NameENUS       string
    SpellIcon      sql.NullInt64
    ClassMask      sql.NullInt64
    CreatureFamily sql.NullInt64
    OrderIndex     sql.NullInt64
    Background     sql.NullString
    OtherLanguage  map[string]sql.NullString
}

type Talent struct {
    ID                int
    SpecID            sql.NullInt64
    TierID            sql.NullInt64
    ColumnIndex       sql.NullInt64
    Rank              [9]sql.NullInt64 // rank_1 .. rank_9
    PreReqTalent      [3]sql.NullInt64 // pre_req_talent_1 .. 3
    PreReqRank        [3]sql.NullInt64 // pre_req_rank_1 .. 3
    Flags             sql.NullInt64
    ReqSpellID        sql.NullInt64
    AllowForPetFlags1 sql.NullInt64
    AllowForPetFlags2 sql.NullInt64
}

type ChrClass struct {
    ID       int
    NameENUS string
    PetName  string
}

type Spell struct {
    ID       int
    NameENUS string
    IconID   sql.NullInt64
    Desc     string
}

type AppContext struct {
    DB              *sql.DB
    GridContainer   *fyne.Container
    EditorContainer *fyne.Container
    Window          fyne.Window
    
    // Caches
    SpellIcons map[int]string
    Spells     map[int]Spell
}

func main() {
    a := app.New()
    window := a.NewWindow("WoW 3.3.5 Talent Editor - MySQL")
    window.Resize(fyne.NewSize(1000, 1080))

    // Load config
    cfgPath := "config.json"
    cfg, created, err := loadOrInitConfig(cfgPath)
    if err != nil {
        dialog.ShowError(fmt.Errorf("failed to load config: %w", err), window)
        return
    }
    if created {
        dialog.ShowInformation("Info", fmt.Sprintf("Template config.json created at %s. Please edit it and restart.", cfgPath), window)
        return
    }

    // Open DB connection
    db, err := openDB(cfg.DBC)
    if err != nil {
        dialog.ShowError(fmt.Errorf("failed to open DB: %w", err), window)
        return
    }
    defer db.Close()

    // Add theme selector config
    a.Settings().SetTheme(&customTheme{base: theme.DefaultTheme(), variant: theme.VariantDark})

    // Left: talent tabs list
    tabsList := widget.NewList(
        func() int { return 0 },
        func() fyne.CanvasObject { return widget.NewLabel("placeholder") },
        func(i widget.ListItemID, o fyne.CanvasObject) {},
    )

    // Center: Talent grid
    gridContainer := container.NewVBox(widget.NewLabel("Select a TalentTab from the left"))
    
    // Right: Talent editor
    editorContainer := container.NewVBox(widget.NewLabel("Select a talent cell to edit"))
    
    // Format column labels with spacing to enforce min col width
    formatLabel:= func(text string, padding int) *widget.Label {
        spaces := strings.Repeat(" ", padding)
        lbl := widget.NewLabel(spaces + text + spaces)
        
        lbl.TextStyle = fyne.TextStyle{
            Bold:   true,
        }
        
        lbl.SizeName = "subHeadingText"
        
        return lbl
    }
    
    talentTabLabel := formatLabel("Talent Tabs", 10)
    editorLabel    := formatLabel("Editor Pane", 30)
    gridLabel      := formatLabel("Talent Grid", 30)
    
    // Enforce minimum column width by abusing labels.. Actually disgusting, but whatever.
    mainContainer := container.NewBorder(
        nil,
        nil,
        container.NewMax(container.NewBorder(container.NewCenter(talentTabLabel), nil, nil, nil, tabsList)),
        container.NewMax(container.NewBorder(container.NewCenter(editorLabel), nil, nil, nil, editorContainer)),
        container.NewMax(container.NewBorder(container.NewCenter(gridLabel), nil, nil, nil, gridContainer)),
    )
    window.SetContent(fynetooltip.AddWindowToolTipLayer(mainContainer, window.Canvas()))
    
    ctx := &AppContext{
        DB:              db,
        GridContainer:   gridContainer,
        EditorContainer: editorContainer,
        Window:          window,
    }
    
    loadTabs(ctx, tabsList)
    window.ShowAndRun()
}

func loadTabs(ctx *AppContext, tabsList *widget.List) {
    tabs, err := GetAllTalentTabs(ctx)
    if err != nil {
        dialog.ShowError(err, ctx.Window)
        return
    }

    // Load classes
    classMap, err := GetAllClasses(ctx)
    if err != nil {
        dialog.ShowError(err, ctx.Window)
        return
    }

    // Group tabs by class
    classTabs := make(map[string][]TalentTab)
    var petTabs []TalentTab
    for _, t := range tabs {
        if t.CreatureFamily.Valid && t.CreatureFamily.Int64 > 0 {
            petTabs = append(petTabs, t)
            continue
        }
        assigned := false
        if t.ClassMask.Valid {
            mask := t.ClassMask.Int64
            for _, class := range classMap {
                if mask&(1<<(class.ID-1)) != 0 {
                    classTabs[class.NameENUS] = append(classTabs[class.NameENUS], t)
                    assigned = true
                }
            }
        }
        if !assigned {
            classTabs["Unknown"] = append(classTabs["Unknown"], t)
        }
    }

    // Flatten all tabs for display
    var displayTabs []struct {
        Group string
        Tab   TalentTab
    }

    // Add all non-pet tabs first
    for className, tabs := range classTabs {
        for _, t := range tabs {
            displayTabs = append(displayTabs, struct {
                Group string
                Tab   TalentTab
            }{Group: className, Tab: t})
        }
    }

    // Append pet tabs at the bottom
    for _, t := range petTabs {
        displayTabs = append(displayTabs, struct {
            Group string
            Tab   TalentTab
        }{Group: "Pet", Tab: t})
    }

    // Sort entire displayTabs slice by Tab.ID
    sort.Slice(displayTabs, func(i, j int) bool {
        // Pets go after non-pets
        if displayTabs[i].Group == "Pet" && displayTabs[j].Group != "Pet" {
            return false
        }
        if displayTabs[i].Group != "Pet" && displayTabs[j].Group == "Pet" {
            return true
        }
        return displayTabs[i].Tab.ID < displayTabs[j].Tab.ID
    })

    // Update the talent tab list
    tabsList.Length = func() int { return len(displayTabs) }
    tabsList.CreateItem = func() fyne.CanvasObject { return widget.NewLabel("") }
    tabsList.UpdateItem = func(i widget.ListItemID, o fyne.CanvasObject) {
        item := displayTabs[i]
        o.(*widget.Label).SetText(fmt.Sprintf("[%s] %s", item.Group, item.Tab.NameENUS))
    }
    tabsList.OnSelected = func(id widget.ListItemID) {
        if id < 0 || id >= len(displayTabs) {
            return
        }
        tab := displayTabs[id].Tab
        loadTalentsForTab(ctx, tab)
    }
    tabsList.Refresh()
}

// loadTalentsForTab queries talents with spec_id = tab.ID and builds a visual grid
func loadTalentsForTab(ctx *AppContext, tab TalentTab) {
    // Clear previous content
    ctx.GridContainer.Objects = nil
    resetEditorContainer(ctx)

    // Load talents from DB
    talents, talentSpellIds, err := GetTalentsForSpec(ctx, tab.ID)
    if err != nil {
        dialog.ShowError(err, ctx.Window)
        return
    }

    // Load all relevant Spells from the talents
    spells, err := GetSpellsByIDs(ctx, talentSpellIds)
    if err != nil {
        dialog.ShowError(err, ctx.Window)
        return
    }

    // Map talents into grid
    grid := mapTalentsToGrid(talents, MAX_NUM_TALENT_TIERS, NUM_TALENT_COLUMNS)

    // Load Spell Icons
    iconIDs, err := GetAllSpellIcons(ctx)
    if err != nil {
        dialog.ShowError(err, ctx.Window)
        return
    }

    // Build talent buttons using custom grid layout
    iconSize := 46
    buttonSize := fyne.NewSize(float32(iconSize), float32(iconSize))
    buttonMap := make(map[int]*TalentButton)
    talentMap := make(map[int]*Talent)

    gridLayout := &grid4x15{
        Rows: MAX_NUM_TALENT_TIERS,
        Cols: NUM_TALENT_COLUMNS,
        HorizontalPad: float32(iconSize/2),
        VerticalPad:   float32(iconSize/2),
    }
    gridWrapper := container.New(gridLayout)

    for r := 0; r < MAX_NUM_TALENT_TIERS; r++ {
        for c := 0; c < NUM_TALENT_COLUMNS; c++ {
            t := grid[r][c]
            tb := createTalentButton(ctx, tab, t, r, c, iconIDs, buttonSize, spells,
                func() { loadTalentsForTab(ctx, tab) })

            gridWrapper.Add(tb)
            
            if t != nil {
                buttonMap[t.ID] = tb
                talentMap[t.ID] = t
            }
        }
    }

    // --- Draw arrows between talents ---
    drawTalentArrows(gridWrapper, buttonMap, talentMap)

    ctx.GridContainer.Add(container.NewCenter(gridWrapper))
    ctx.GridContainer.Refresh()
}

func mapTalentsToGrid(talents []Talent, rows, cols int) [][]*Talent {
    grid := make([][]*Talent, rows)
    for r := range grid {
        grid[r] = make([]*Talent, cols)
    }

    for i := range talents {
        t := &talents[i]
        r, c := 0, 0
        if t.TierID.Valid {
            r = int(t.TierID.Int64)
        }
        if t.ColumnIndex.Valid {
            c = int(t.ColumnIndex.Int64)
        }
        if r >= 0 && r < rows && c >= 0 && c < cols {
            grid[r][c] = t
        }
    }

    return grid
}

// openTalentEditor shows a form for viewing/editing a Talent
func openTalentEditor(ctx *AppContext, t *Talent, isNew bool, reloadTab func()) {
    ctx.EditorContainer.Objects = nil
    const entryWidth = 150

    // --- Helpers ---
    makeEntry := func(def string) *widget.Entry {
        e := widget.NewEntry()
        e.SetText(def)
        return e
    }

    useLabel := func(val string) fyne.CanvasObject { 
        lbl := widget.NewLabel(val) 
        fixed := container.NewWithoutLayout(lbl) 
        lbl.Resize(fyne.NewSize(entryWidth, lbl.MinSize().Height)) 
        return fixed 
    }

    getIntStr := func(n sql.NullInt64) string {
        if n.Valid {
            return fmt.Sprintf("%d", n.Int64)
        }
        return "0"
    }

    // Map for saving values
    fields := map[string]fyne.CanvasObject{}

    // Helper to create form items
    makeFormItem := func(label string, w fyne.CanvasObject) *widget.FormItem {
        fields[label] = w
        lbl := widget.NewLabel(label)
        lbl.Alignment = fyne.TextAlignLeading
        hbox := container.New(layout.NewGridLayout(2), lbl, w)
        return &widget.FormItem{
            Text:   "",
            Widget: hbox,
        }
    }

    // --- Build form entries ---
    talentID := useLabel(fmt.Sprintf("%d", t.ID))
    specEntry := useLabel(getIntStr(t.SpecID))
    tierEntry := useLabel(getIntStr(t.TierID))
    colEntry := useLabel(getIntStr(t.ColumnIndex))

    rankEntries := make([]*widget.Entry, 9)
    for i := 0; i < 9; i++ {
        rankEntries[i] = makeEntry(getIntStr(t.Rank[i]))
    }

    preTalentEntries := make([]*widget.Entry, 3)
    preRankEntries := make([]*widget.Entry, 3)
    for i := 0; i < 3; i++ {
        preTalentEntries[i] = makeEntry(getIntStr(t.PreReqTalent[i]))
        preRankEntries[i] = makeEntry(getIntStr(t.PreReqRank[i]))
    }

    flagsEntry := makeEntry(getIntStr(t.Flags))
    reqSpellEntry := makeEntry(getIntStr(t.ReqSpellID))
    allowPet1Entry := makeEntry(getIntStr(t.AllowForPetFlags1))
    allowPet2Entry := makeEntry(getIntStr(t.AllowForPetFlags2))

    // --- Build form items ---
    formItems := []*widget.FormItem{
        makeFormItem("Talent ID", talentID),
        makeFormItem("Spec ID", specEntry),
        makeFormItem("Tier ID", tierEntry),
        makeFormItem("Column Index", colEntry),
    }

    for i := 0; i < 9; i++ {
        formItems = append(formItems, makeFormItem(fmt.Sprintf("Rank %d", i+1), rankEntries[i]))
    }
    for i := 0; i < 3; i++ {
        formItems = append(formItems, makeFormItem(fmt.Sprintf("Pre-requisite Talent ID %d", i+1), preTalentEntries[i]))
        formItems = append(formItems, makeFormItem(fmt.Sprintf("Pre-requisite Rank %d", i+1), preRankEntries[i]))
    }
    formItems = append(formItems,
        makeFormItem("Flags", flagsEntry),
        makeFormItem("Required Spell ID", reqSpellEntry),
        makeFormItem("Allow for Pet Flags 1", allowPet1Entry),
        makeFormItem("Allow for Pet Flags 2", allowPet2Entry),
    )

    form := widget.NewForm(formItems...)
    formFiller := container.NewMax(form)

    // --- Build buttons ---
    saveBtn := widget.NewButton("Save", func() {
        saveTalentHandler(ctx, t, isNew, fields, reloadTab)
    })
    cancelBtn := widget.NewButton("Cancel", func() {
        resetEditorContainer(ctx)
    })
    deleteBtn := widget.NewButton("Delete", func() {
        deleteTalentHandler(ctx, t, reloadTab)
    })
    deleteBtn.Importance = widget.DangerImportance

    var btnRow fyne.CanvasObject
    if isNew {
        btnRow = NewButtonRow(container.NewHBox(saveBtn, cancelBtn), nil)
    } else {
        btnRow = NewButtonRow(container.NewHBox(saveBtn, cancelBtn), deleteBtn)
    }

    ctx.EditorContainer.Objects = []fyne.CanvasObject{
        container.NewBorder(formFiller, btnRow, nil, nil, container.NewMax()),
    }
    ctx.EditorContainer.Refresh()
}


// Fixed-height button row with left/right layout
func NewButtonRow(leftButtons fyne.CanvasObject, rightButton fyne.CanvasObject) *fyne.Container {
    bg := canvas.NewRectangle(color.NRGBA{A: 0}) // transparent background
    height := float32(40)
    bg.SetMinSize(fyne.NewSize(0, height))

    // Use a Border layout: leftButtons on left, rightButton on right, center empty
    row := container.NewBorder(nil, nil, leftButtons, rightButton, container.NewMax())

    return container.NewMax(bg, row)
}

func NewEmptyTalent(specID, tier, col int) *Talent {
    t := &Talent{
        SpecID:      sql.NullInt64{Int64: int64(specID), Valid: true},
        TierID:      sql.NullInt64{Int64: int64(tier), Valid: true},
        ColumnIndex: sql.NullInt64{Int64: int64(col), Valid: true},
        Flags:       sql.NullInt64{Int64: 0, Valid: true},
        ReqSpellID:  sql.NullInt64{Int64: 0, Valid: true},
        AllowForPetFlags1: sql.NullInt64{Int64: 0, Valid: true},
        AllowForPetFlags2: sql.NullInt64{Int64: 0, Valid: true},
    }
    for i := 0; i < 9; i++ {
        t.Rank[i] = sql.NullInt64{Int64: 0, Valid: true}
    }
    for i := 0; i < 3; i++ {
        t.PreReqTalent[i] = sql.NullInt64{Int64: 0, Valid: true}
        t.PreReqRank[i]   = sql.NullInt64{Int64: 0, Valid: true}
    }
    return t
}

func NewTransparentIconWithBorder(width, height int, borderColor color.NRGBA, dashLength int) fyne.Resource {
    img := image.NewRGBA(image.Rect(0, 0, width, height))

    // Fill the interior with nearly transparent color (alpha=1)
    for y := 0; y < height; y++ {
        for x := 0; x < width; x++ {
            img.Set(x, y, color.NRGBA{R: 0, G: 0, B: 0, A: 1})
        }
    }

    // Draw dashed border (top & bottom)
    for x := 0; x < width; x += dashLength * 2 {
        for dx := 0; dx < dashLength && x+dx < width; dx++ {
            img.Set(x+dx, 0, borderColor)        // top
            img.Set(x+dx, height-1, borderColor) // bottom
        }
    }

    // Draw dashed border (left & right)
    for y := 0; y < height; y += dashLength * 2 {
        for dy := 0; dy < dashLength && y+dy < height; dy++ {
            img.Set(0, y+dy, borderColor)        // left
            img.Set(width-1, y+dy, borderColor)  // right
        }
    }

    var buf bytes.Buffer
    png.Encode(&buf, img)
    return fyne.NewStaticResource("transparent_border.png", buf.Bytes())
}

func createArrowHead(x1, y1, x2, y2 float32, size float32) []fyne.CanvasObject {
    dx := x2 - x1
    dy := y2 - y1
    length := float32(math.Hypot(float64(dx), float64(dy)))
    if length == 0 {
        return nil
    }

    ux := dx / length
    uy := dy / length

    // Perpendicular vector
    px := -uy
    py := ux

    leftX := x2 - ux*size + px*size/2
    leftY := y2 - uy*size + py*size/2

    rightX := x2 - ux*size - px*size/2
    rightY := y2 - uy*size - py*size/2

    leftLine := canvas.NewLine(color.NRGBA{R: 255, G: 0, B: 0, A: 255})
    leftLine.StrokeWidth = 2
    leftLine.Position1 = fyne.NewPos(x2, y2)
    leftLine.Position2 = fyne.NewPos(leftX, leftY)

    rightLine := canvas.NewLine(color.NRGBA{R: 255, G: 0, B: 0, A: 255})
    rightLine.StrokeWidth = 2
    rightLine.Position1 = fyne.NewPos(x2, y2)
    rightLine.Position2 = fyne.NewPos(rightX, rightY)

    return []fyne.CanvasObject{leftLine, rightLine}
}

// Draw arrows between talents based on prerequisites
func drawTalentArrows(gridWrapper *fyne.Container, buttonMap map[int]*TalentButton, talentMap map[int]*Talent) {
    for id, btn := range buttonMap {
        t, ok := talentMap[id]
        if !ok || t == nil {
            continue
        }

        for i := 0; i < 3; i++ {
            if !t.PreReqTalent[i].Valid || t.PreReqTalent[i].Int64 == 0 {
                continue
            }
            preID := int(t.PreReqTalent[i].Int64)

            preBtn, ok := buttonMap[preID]
            if !ok || preBtn == nil {
                continue
            }

            parentX := preBtn.Position().X
            parentY := preBtn.Position().Y
            parentW := preBtn.Size().Width
            parentH := preBtn.Size().Height

            childX := btn.Position().X
            childY := btn.Position().Y
            childW := btn.Size().Width

            // Determine arrow type
            if parentY == childY {
                drawHorizontalArrow(gridWrapper, parentX, parentY, parentW, childX, childW)
            } else if parentX == childX {
                drawVerticalArrow(gridWrapper, parentX, parentY, parentH, childY)
            } else {
                drawStepArrow(gridWrapper, parentX, parentY, parentW, parentH, childX, childY, childW)
            }
        }
    }
    gridWrapper.Refresh()
}

// Horizontal arrow on same row
func drawHorizontalArrow(gridWrapper *fyne.Container, parentX, parentY, parentW, childX, childW float32) {
    startX := parentX + parentW/2
    endX := childX + childW/2
    startY := parentY + parentW/2
    endY := startY

    if endX > startX {
        startX = parentX + parentW
        endX = childX
    } else {
        startX = parentX
        endX = childX + childW
    }

    line := canvas.NewLine(color.NRGBA{R: 255, G: 0, B: 0, A: 255})
    line.StrokeWidth = 2
    line.Position1 = fyne.NewPos(startX, startY)
    line.Position2 = fyne.NewPos(endX, endY)
    gridWrapper.Add(line)

    for _, ah := range createArrowHead(startX, startY, endX, endY, 8) {
        gridWrapper.Add(ah)
    }
}

// Vertical arrow on same column
func drawVerticalArrow(gridWrapper *fyne.Container, parentX, parentY, parentH, childY float32) {
    startX := parentX + parentH/2
    startY := parentY + parentH
    endX := startX
    endY := childY

    line := canvas.NewLine(color.NRGBA{R: 255, G: 0, B: 0, A: 255})
    line.StrokeWidth = 2
    line.Position1 = fyne.NewPos(startX, startY)
    line.Position2 = fyne.NewPos(endX, endY)
    gridWrapper.Add(line)

    for _, ah := range createArrowHead(startX, startY, endX, endY, 8) {
        gridWrapper.Add(ah)
    }
}

// Step-wise arrow: horizontal then vertical
func drawStepArrow(gridWrapper *fyne.Container, parentX, parentY, parentW, parentH, childX, childY, childW float32) {
    startX := parentX
    if childX > parentX {
        startX = parentX + parentW // exit right
    }

    startY := parentY + parentH/2
    endX := childX + childW/2
    endY := childY

    // Horizontal segment
    lineH := canvas.NewLine(color.NRGBA{R: 255, G: 0, B: 0, A: 255})
    lineH.StrokeWidth = 2
    lineH.Position1 = fyne.NewPos(startX, startY)
    lineH.Position2 = fyne.NewPos(endX, startY)
    gridWrapper.Add(lineH)

    // Vertical segment
    lineV := canvas.NewLine(color.NRGBA{R: 255, G: 0, B: 0, A: 255})
    lineV.StrokeWidth = 2
    lineV.Position1 = fyne.NewPos(endX, startY)
    lineV.Position2 = fyne.NewPos(endX, endY)
    gridWrapper.Add(lineV)

    // Arrowhead
    for _, ah := range createArrowHead(endX, startY, endX, endY, 8) {
        gridWrapper.Add(ah)
    }
}

func createTalentButton(
    ctx *AppContext,
    tab TalentTab,
    talent *Talent,
    row, column int,
    iconIDs map[int]string,
    buttonSize fyne.Size,
    spells map[int]Spell,
    reloadTab func(),
) *TalentButton {
    iconResource := theme.BrokenImageIcon()
    tooltip := ""
    var onTap func()

    if talent == nil {
        iconResource = NewTransparentIconWithBorder(
            int(buttonSize.Width),
            int(buttonSize.Height),
            color.NRGBA{R: 255, G: 255, B: 255, A: 255},
            4,
        )
        tooltip = "Empty talent slot"
        onTap = func() {
            emptyTalent := NewEmptyTalent(tab.ID, row, column)
            openTalentEditor(ctx, emptyTalent, true, reloadTab)
        }
    } else {
        if talent.Rank[0].Valid {
            rankSpellID := int(talent.Rank[0].Int64)
            if spell, ok := spells[rankSpellID]; ok && spell.IconID.Valid {
                if iconFile, ok := iconIDs[int(spell.IconID.Int64)]; ok {
                    name := strings.ToLower(iconFile + ".png")
                    if actual, ok := iconLookup[name]; ok {
                        if data, err := fs.ReadFile(iconsFS, actual); err == nil {
                            iconResource = fyne.NewStaticResource(actual, data)
                        }
                    }
                }
                tooltip = fmt.Sprintf("%s\nID: %d\n%s", spell.NameENUS, spell.ID, spell.Desc)
            }
        }
        tRef := talent
        onTap = func() {
            openTalentEditor(ctx, tRef, false, reloadTab)
        }
    }

    return NewTalentButton(iconResource, buttonSize, tooltip, onTap)
}

func saveTalentHandler(ctx *AppContext, talent *Talent, isNew bool, formFields map[string]fyne.CanvasObject, reloadTab func()) {
    parseEntry := func(label string) sql.NullInt64 {
        w, ok := formFields[label]
        if !ok {
            return sql.NullInt64{Valid: false}
        }

        switch v := w.(type) {
        case *widget.Entry:
            s := strings.TrimSpace(v.Text)
            if s == "" {
                return sql.NullInt64{Valid: false}
            }
            n, err := strconv.ParseInt(s, 10, 64)
            if err != nil {
                return sql.NullInt64{Valid: false}
            }
            return sql.NullInt64{Int64: n, Valid: true}
        case *widget.Label:
            s := strings.TrimSpace(v.Text)
            if s == "" {
                return sql.NullInt64{Valid: false}
            }
            n, err := strconv.ParseInt(s, 10, 64)
            if err != nil {
                return sql.NullInt64{Valid: false}
            }
            return sql.NullInt64{Int64: n, Valid: true}
        default:
            return sql.NullInt64{Valid: false}
        }
    }

    for i := 0; i < 9; i++ {
        talent.Rank[i] = parseEntry(fmt.Sprintf("Rank %d", i+1))
    }
    for i := 0; i < 3; i++ {
        talent.PreReqTalent[i] = parseEntry(fmt.Sprintf("Pre-requisite Talent ID %d", i+1))
        talent.PreReqRank[i] = parseEntry(fmt.Sprintf("Pre-requisite Rank %d", i+1))
    }

    talent.Flags = parseEntry("Flags")
    talent.ReqSpellID = parseEntry("Required Spell ID")
    talent.AllowForPetFlags1 = parseEntry("Allow for Pet Flags 1")
    talent.AllowForPetFlags2 = parseEntry("Allow for Pet Flags 2")

    var err error
    if isNew {
        err = insertTalent(ctx, talent)
    } else {
        err = updateTalent(ctx, talent)
    }

    if err != nil {
        dialog.ShowError(err, ctx.Window)
        return
    }

    reloadTab()
    resetEditorContainer(ctx)
}

func deleteTalentHandler(ctx *AppContext, talent *Talent, reloadTab func()) {
    confirm := dialog.NewConfirm("Confirm Delete", "Are you sure you want to delete this talent?", func(yes bool) {
        if !yes {
            return
        }
        if err := deleteTalent(ctx, talent); err != nil {
            dialog.ShowError(err, ctx.Window)
            return
        }
        reloadTab()
        resetEditorContainer(ctx)
    }, ctx.Window)
    confirm.Show()
}

// updateTalent performs an UPDATE against the Talent table for existing id
func updateTalent(ctx *AppContext, talent *Talent) error {
    query, args := UpdateTalentQuery(talent)
    _, err := execWithDebug(ctx.DB, query, args...)
    return err
}

// insertTalent performs an INSERT into Talent
func insertTalent(ctx *AppContext, talent *Talent) error {
    if talent.ID == 0 {
        var maxID int64
        err := ctx.DB.QueryRow("SELECT COALESCE(MAX(id), 0) FROM Talent").Scan(&maxID)
        if err != nil {
            return err
        }
        talent.ID = int(maxID + 1)
    }
    query, args := InsertTalentQuery(talent)
    _, err := execWithDebug(ctx.DB, query, args...)
    return err
}

func deleteTalent(ctx *AppContext, talent *Talent) error {
    if talent == nil || talent.ID == 0 {
        return fmt.Errorf("invalid talent")
    }
    query, args := DeleteTalentQuery(talent.ID)
    _, err := execWithDebug(ctx.DB, query, args...)
    return err
}

func resetEditorContainer(ctx *AppContext) {
    ctx.EditorContainer.Objects = nil
    ctx.EditorContainer.Add(widget.NewLabel("Select a talent cell to edit"))
    ctx.EditorContainer.Refresh()
}