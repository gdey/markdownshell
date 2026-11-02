-- mdsh-gui's side of the embedded Neovim: the buffer holding the file,
-- kept in step with mdsh-gui over RPC. Run once with nvim_exec_lua, with
-- the channel to mdsh-gui, the buffer's name, its lines and the command
-- that starts mdsh's language server.

local chan, name, lines, lsp_cmd = ...

local group = vim.api.nvim_create_augroup("mdsh", { clear = true })
local buf = vim.api.nvim_create_buf(true, false)
-- No swap file: mdsh-gui keeps the text, and a stale one would stop
-- Neovim with a prompt.
vim.bo[buf].swapfile = false
vim.api.nvim_buf_set_name(buf, name)

-- applying is true while mdsh-gui sets the text, so the change isn't
-- sent back.
local applying = false

local function set_lines(l)
  applying = true
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, l)
  applying = false
end

-- The first text isn't an edit: u shouldn't empty the buffer.
local ul = vim.bo[buf].undolevels
vim.bo[buf].undolevels = -1
set_lines(lines)
vim.bo[buf].undolevels = ul
vim.bo[buf].buftype = "acwrite"
vim.bo[buf].fileformat = "unix"
vim.bo[buf].filetype = "markdown"
vim.bo[buf].modified = false

local function show()
  if vim.api.nvim_buf_is_valid(buf) then
    vim.api.nvim_set_current_buf(buf)
  end
end
show()

-- With --embed the user's config runs once the UI attaches, which may be
-- after this; a config that opens its own buffer shouldn't hide ours.
vim.api.nvim_create_autocmd("VimEnter", { group = group, once = true, callback = show })

-- :w hands the text to mdsh-gui, which saves the file.
vim.api.nvim_create_autocmd("BufWriteCmd", {
  group = group,
  buffer = buf,
  callback = function()
    vim.rpcnotify(chan, "mdsh_write", table.concat(vim.api.nvim_buf_get_lines(buf, 0, -1, false), "\n") .. "\n")
  end,
})

-- Typing sends the whole text after a short pause, as the CodeMirror
-- source pane does.
local timer = vim.uv.new_timer()
local function send()
  if vim.api.nvim_buf_is_valid(buf) then
    vim.rpcnotify(chan, "mdsh_text", table.concat(vim.api.nvim_buf_get_lines(buf, 0, -1, false), "\n") .. "\n")
  end
end
vim.api.nvim_buf_attach(buf, false, {
  on_lines = function()
    if applying then
      return
    end
    timer:stop()
    timer:start(300, 0, vim.schedule_wrap(send))
  end,
})

-- mdsh is what mdsh-gui calls into.
_G.mdsh = {
  -- set_text replaces the text, as after a run writes its output,
  -- keeping the view where it was.
  set_text = function(l)
    local wins = vim.fn.win_findbuf(buf)
    local views = {}
    for _, w in ipairs(wins) do
      views[w] = vim.api.nvim_win_call(w, vim.fn.winsaveview)
    end
    set_lines(l)
    for w, v in pairs(views) do
      vim.api.nvim_win_call(w, function() vim.fn.winrestview(v) end)
    end
  end,
  -- saved marks the buffer unchanged once mdsh-gui wrote the file.
  saved = function()
    vim.bo[buf].modified = false
  end,
}

-- apply makes edits, each replacing lines [start, end), counted from 0,
-- that hold old with new, last first. If any edit's lines don't hold old
-- any more, as when typing moved them, it changes nothing and returns
-- false: mdsh-gui then sets the whole text.
function mdsh.apply(edits)
  for _, e in ipairs(edits) do
    if not vim.deep_equal(vim.api.nvim_buf_get_lines(buf, e.start, e["end"], false), e.old or {}) then
      return false
    end
  end
  applying = true
  for i = #edits, 1, -1 do
    local e = edits[i]
    vim.api.nvim_buf_set_lines(buf, e.start, e["end"], false, e.new or {})
  end
  applying = false
  return true
end

-- What mdsh-gui hears from the buffer, as "mdsh_event" notifications
-- with JSON data.
local function notify(name, data)
  vim.rpcnotify(chan, "mdsh_event", name, vim.json.encode(data or vim.empty_dict()))
end

-- win is the window showing the buffer, or nil.
local function win()
  local cur = vim.api.nvim_get_current_win()
  if vim.api.nvim_win_get_buf(cur) == buf then
    return cur
  end
  return vim.fn.win_findbuf(buf)[1]
end

-- calls are what mdsh-gui calls with mdsh.call, taking and returning
-- JSON, so neither side minds msgpack's types.
local calls = {}

function mdsh.call(fn, args)
  local f = calls[fn]
  if not f then
    error("mdsh: no call " .. fn)
  end
  local out = f(unpack(vim.json.decode(args)))
  if out == nil then
    return "null"
  end
  return vim.json.encode(out)
end

-- state is the text as it is now, sent at once rather than after the
-- pause typing waits for, with the mode, the cursor and, in visual
-- mode, the other end of the selection. Lines count from 1 and columns
-- are bytes from 0.
function calls.state()
  local w = win()
  local st = {
    text = table.concat(vim.api.nvim_buf_get_lines(buf, 0, -1, false), "\n") .. "\n",
    mode = vim.api.nvim_get_mode().mode,
  }
  if w then
    local c = vim.api.nvim_win_get_cursor(w)
    st.line, st.col = c[1], c[2]
    if vim.api.nvim_get_current_win() == w and st.mode:match("^[vV\22]") then
      local v = vim.fn.getpos("v")
      st.vline, st.vcol = v[2], v[3] - 1
    end
  end
  return st
end

-- jump scrolls line to the window's top less rows, and with cursor puts
-- the cursor on it, at col (bytes from 0) or the line's end if col is -1.
function calls.jump(line, rows, cursor, col)
  local w = win()
  if not w then
    return
  end
  local n = vim.api.nvim_buf_line_count(buf)
  line = math.max(1, math.min(line, n))
  vim.api.nvim_win_call(w, function()
    if cursor then
      local text = vim.api.nvim_buf_get_lines(buf, line - 1, line, false)[1] or ""
      if col < 0 then
        col = math.max(0, #text - 1)
      end
      vim.api.nvim_win_set_cursor(w, { line, col })
      vim.cmd("normal! zv")
    end
    local top = math.max(1, line - math.max(0, rows))
    local view = vim.fn.winsaveview()
    view.topline = top
    if not cursor then
      -- The cursor stays put unless it would leave the window.
      view.lnum, view.col = nil, nil
    end
    vim.fn.winrestview(view)
  end)
end

-- select selects from (line, col) to (line, col) in select mode, so
-- typing replaces it, as for the id in a block put in from the Insert
-- menu; an empty selection just puts the cursor there. Lines count from
-- 1 and columns are bytes from 0; the end is just past the selection.
function calls.select(aline, acol, hline, hcol)
  local w = win()
  if not w then
    return
  end
  vim.api.nvim_set_current_win(w)
  local mode = vim.api.nvim_get_mode().mode
  if mode ~= "n" then
    vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes("<Esc>", true, false, true), "nx", false)
  end
  vim.api.nvim_win_set_cursor(w, { aline, acol })
  if aline == hline and acol == hcol then
    return
  end
  -- Visual mode takes in the character under the cursor: end on the
  -- last one selected.
  local text = vim.api.nvim_buf_get_lines(buf, hline - 1, hline, false)[1] or ""
  local last = hcol > 0 and vim.str_utf_start(text, hcol) + hcol - 1 or 0
  vim.cmd("normal! v")
  vim.api.nvim_win_set_cursor(w, { hline, math.max(0, last) })
  vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes("<C-g>", true, false, true), "n", false)
end

-- Run buttons: a ▶ in the sign column on each block that runs, and a ⏵
-- on each heading whose section has one. A click on one runs it, as
-- :RunBlock and :RunSection run the block or section the cursor is in.
local runs = vim.api.nvim_create_namespace("mdsh_runs")
local run_lines = {}

function calls.set_runs(blocks, sections)
  vim.api.nvim_buf_clear_namespace(buf, runs, 0, -1)
  run_lines = {}
  local n = vim.api.nvim_buf_line_count(buf)
  local function mark(line, text, title)
    if line >= 1 and line <= n then
      vim.api.nvim_buf_set_extmark(buf, runs, line - 1, 0, { sign_text = text, sign_hl_group = "mdshRun" })
      run_lines[line] = title
    end
  end
  for _, l in ipairs(sections) do
    mark(l, "⏵", "section")
  end
  for _, l in ipairs(blocks) do
    mark(l, "▶", "block")
  end
  local w = win()
  if w and vim.wo[w].signcolumn == "auto" then
    vim.wo[w].signcolumn = "yes"
  end
end
vim.api.nvim_set_hl(0, "mdshRun", { default = true, link = "DiagnosticOk" })

-- run_mark is the line of the run button under the mouse, or nil. The
-- marks move with the text, so they are read back from the buffer.
local function run_mark()
  local m = vim.fn.getmousepos()
  if m.winid == 0 or vim.api.nvim_win_get_buf(m.winid) ~= buf or m.line == 0 then
    return nil
  end
  -- The sign column is after the fold column.
  local fc = vim.wo[m.winid].foldcolumn
  local fold = tonumber(fc:match("%d+$")) or 0
  if fc:match("^auto") then
    fold = vim.fn.foldlevel(m.line) > 0 and math.min(fold, vim.fn.foldlevel(m.line)) or 0
  end
  local sc = vim.wo[m.winid].signcolumn
  if sc == "no" or sc == "number" or m.wincol <= fold or m.wincol > fold + 2 then
    return nil
  end
  local marks = vim.api.nvim_buf_get_extmarks(buf, runs, { m.line - 1, 0 }, { m.line - 1, -1 }, {})
  if #marks == 0 then
    return nil
  end
  return m.line
end

vim.keymap.set({ "n", "i", "v" }, "<LeftMouse>", function()
  local line = run_mark()
  if not line then
    return "<LeftMouse>"
  end
  notify("run", { line = line })
  return ""
end, { buffer = buf, expr = true, desc = "mdsh: run the block or section" })

vim.api.nvim_buf_create_user_command(buf, "RunBlock", function()
  notify("run", { what = "block" })
end, { desc = "Run the block the cursor is in" })
vim.api.nvim_buf_create_user_command(buf, "RunSection", function()
  notify("run", { what = "section" })
end, { desc = "Run the section the cursor is in" })

-- The cursor's line and the window's top line go to mdsh-gui, which
-- marks the cell the cursor is in and keeps the rendered view in step.
local last_line, last_top = 0, 0
vim.api.nvim_create_autocmd({ "CursorMoved", "CursorMovedI" }, {
  group = group,
  buffer = buf,
  callback = function()
    local line = vim.api.nvim_win_get_cursor(0)[1]
    if line ~= last_line then
      last_line = line
      notify("cursor", { line = line })
    end
  end,
})
vim.api.nvim_create_autocmd("WinScrolled", {
  group = group,
  callback = function()
    local w = win()
    if not w then
      return
    end
    local top = vim.fn.line("w0", w)
    if top ~= last_top then
      last_top = top
      notify("scroll", { line = top })
    end
  end,
})

-- Spelling and the background follow mdsh-gui's settings.
function calls.set_spell(on)
  for _, w in ipairs(vim.fn.win_findbuf(buf)) do
    vim.wo[w].spell = on
  end
end

function calls.set_background(bg)
  if vim.o.background ~= bg then
    vim.o.background = bg
  end
end

-- mdsh's language server completes a fence's attributes and depends=
-- names, explains the attribute under the cursor (K, Neovim's hover) and
-- reports mistakes in them as diagnostics.
local function fence_attrs(text, col)
  local before = text:sub(1, col)
  return before:match("^%s*[>%s]*[-*+%d.)%s]*[`~][`~][`~]+[^{]*{[^}]*$") ~= nil
end

if lsp_cmd and #lsp_cmd > 0 then
  vim.lsp.start({
    name = "mdsh",
    cmd = lsp_cmd,
    root_dir = vim.fn.fnamemodify(lsp_cmd[3] or ".", ":h"),
  }, { bufnr = buf, reuse_client = function(c, conf) return c.name == conf.name end })
end

-- Without a completion plugin, typing in a fence's {…} opens the
-- server's completions, as the built-in editor does. A plugin such as
-- nvim-cmp or blink.cmp asks the server itself.
local function has_plugin()
  return package.loaded["cmp"] ~= nil or package.loaded["blink.cmp"] ~= nil or package.loaded["mini.completion"] ~= nil
end

vim.api.nvim_create_autocmd("TextChangedI", {
  group = group,
  buffer = buf,
  callback = function()
    if has_plugin() or vim.fn.pumvisible() == 1 or vim.bo[buf].omnifunc == "" then
      return
    end
    local c = vim.api.nvim_win_get_cursor(0)
    local text = vim.api.nvim_get_current_line()
    if c[2] == 0 or not fence_attrs(text, c[2]) or not text:sub(c[2], c[2]):match('[%w#<=,"_{%s-]') then
      return
    end
    vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes("<C-x><C-o>", true, false, true), "n", false)
  end,
})

-- While typing in the buffer, that menu doesn't pick or put in an item
-- until asked.
local saved_completeopt
vim.api.nvim_create_autocmd("InsertEnter", {
  group = group,
  buffer = buf,
  callback = function()
    if has_plugin() then
      return
    end
    saved_completeopt = vim.o.completeopt
    vim.o.completeopt = "menuone,noinsert,noselect"
  end,
})
vim.api.nvim_create_autocmd("InsertLeave", {
  group = group,
  buffer = buf,
  callback = function()
    if saved_completeopt then
      vim.o.completeopt = saved_completeopt
      saved_completeopt = nil
    end
  end,
})

-- Folds: a heading folds its section, down to the next heading of the
-- same or a higher level, and a fence or an mdsh:output region folds to
-- its closing line, as in the built-in editor (fold.ts). The levels are
-- worked out once for each change to the buffer.

local function heading_level(l, n)
  local text = l[n]
  local h = text:match("^ ? ? ?(#+)%s") or text:match("^ ? ? ?(#+)$")
  if h and #h <= 6 then
    return #h
  end
  if n >= #l or not text:match("%S") or text:match("^%s*[-*+]%s") or text:match("^%s*%d+[.)]%s") or text:match("^%s*>") or text:match("^%s*<!%-%-") then
    return 0
  end
  local under = l[n + 1]
  if under:match("^ ? ? ?=+%s*$") then
    return 1
  end
  if under:match("^ ? ? ?%-+%s*$") then
    return 2
  end
  return 0
end

-- outline is, for each line that opens a fold, the last line it hides.
local function outline(l)
  local folds = {}
  local open = {}
  local function close(line, level)
    while #open > 0 and open[#open].level >= level do
      local h = table.remove(open)
      local e = line - 1
      while e > h.line and not l[e]:match("%S") do
        e = e - 1
      end
      if e > h.line then
        folds[h.line] = e
      end
    end
  end
  local n = 1
  while n <= #l do
    local text = l[n]
    local prefix, fence = text:match("^(%s*[>%s]*[-*+%d.)%s]-)([`~][`~][`~]+)")
    if fence and fence:match("^" .. fence:sub(1, 1) .. "+$") then
      local ch = fence:sub(1, 1) == "`" and "`" or "~"
      local e = n + 1
      while e <= #l do
        local f = l[e]:match("^%s*[>%s]*(" .. ch .. "+)%s*$")
        if f and #f >= #fence then
          break
        end
        e = e + 1
      end
      e = math.min(e, #l)
      if e > n then
        folds[n] = e
      end
      n = e + 1
    elseif text:match("^%s*[>%s]*<!%-%- mdsh:output.*%-%->%s*$") then
      local e = n + 1
      while e <= #l and not l[e]:match("^%s*[>%s]*<!%-%- /mdsh:output %-%->%s*$") do
        e = e + 1
      end
      if e <= #l then
        folds[n] = e
        n = e + 1
      else
        n = n + 1
      end
    else
      local level = heading_level(l, n)
      if level > 0 then
        close(n, level)
        open[#open + 1] = { line = n, level = level }
        if not (text:match("^ ? ? ?#")) then
          n = n + 1
        end
      end
      n = n + 1
    end
  end
  close(#l + 1, 0)
  return folds
end

local levels, levels_tick = {}, -1

-- fold_levels gives each line its foldexpr level: how many folds hold
-- it, with ">" where one opens.
local function fold_levels()
  local tick = vim.b[buf].changedtick
  if tick == levels_tick then
    return levels
  end
  local l = vim.api.nvim_buf_get_lines(buf, 0, -1, false)
  local depth = {}
  for i = 1, #l do
    depth[i] = 0
  end
  local starts = {}
  for s, e in pairs(outline(l)) do
    starts[s] = true
    for i = s, e do
      depth[i] = depth[i] + 1
    end
  end
  levels = {}
  for i = 1, #l do
    levels[i] = (starts[i] and ">" or "") .. depth[i]
  end
  levels_tick = tick
  return levels
end

function mdsh.foldexpr(lnum)
  return fold_levels()[lnum] or "0"
end

local function use_folds(w)
  vim.wo[w].foldmethod = "expr"
  vim.wo[w].foldexpr = "v:lua.mdsh.foldexpr(v:lnum)"
  vim.wo[w].foldlevel = 99
end

vim.api.nvim_create_autocmd("BufWinEnter", {
  group = group,
  buffer = buf,
  callback = function()
    use_folds(vim.api.nvim_get_current_win())
  end,
})
for _, w in ipairs(vim.fn.win_findbuf(buf)) do
  use_folds(w)
end

-- set_folds closes the folds opening on these lines and opens the rest.
function calls.set_folds(lines)
  local w = win()
  if not w then
    return
  end
  vim.api.nvim_win_call(w, function()
    vim.cmd("normal! zR")
    table.sort(lines, function(a, b) return a > b end)
    local n = vim.api.nvim_buf_line_count(buf)
    for _, l in ipairs(lines) do
      if l >= 1 and l <= n and fold_levels()[l]:match("^>") then
        vim.cmd(l .. "foldclose")
      end
    end
  end)
end

-- folds are the lines whose folds are closed.
function calls.folds()
  local w = win()
  local out = {}
  if not w then
    return out
  end
  vim.api.nvim_win_call(w, function()
    local n = vim.api.nvim_buf_line_count(buf)
    for l = 1, n do
      if fold_levels()[l]:match("^>") and vim.fn.foldclosed(l) == l then
        out[#out + 1] = l
      end
    end
  end)
  return out
end

-- Colors: the app's theme, from the page's color tokens (style.css), so
-- the pane looks like the built-in editor and follows the Theme setting,
-- unless the setting to use them is turned off.
local function app_colors(c)
  local hl = function(name, spec)
    vim.api.nvim_set_hl(0, name, spec)
  end
  -- Clear what an earlier scheme set, as :colorscheme does.
  vim.cmd("highlight clear")
  vim.o.background = c.dark and "dark" or "light"
  vim.o.termguicolors = true

  hl("Normal", { fg = c.fg, bg = c.code_bg })
  hl("NormalNC", { link = "Normal" })
  hl("NormalFloat", { fg = c.fg, bg = c.bg })
  hl("FloatBorder", { fg = c.border, bg = c.bg })
  hl("FloatTitle", { fg = c.fg, bg = c.bg, bold = true })
  hl("Cursor", { fg = c.bg, bg = c.fg })
  hl("CursorLine", { bg = c.active_line })
  hl("CursorColumn", { bg = c.active_line })
  hl("ColorColumn", { bg = c.active_line })
  hl("CursorLineNr", { fg = c.fg, bg = c.active_line })
  hl("LineNr", { fg = c.muted })
  hl("SignColumn", { fg = c.muted, bg = c.code_bg })
  hl("FoldColumn", { fg = c.muted, bg = c.code_bg })
  hl("Folded", { fg = c.muted, bg = c.hover, italic = true })
  hl("EndOfBuffer", { fg = c.border })
  hl("NonText", { fg = c.border })
  hl("Whitespace", { fg = c.border })
  hl("SpecialKey", { fg = c.muted })
  hl("Visual", { bg = c.selection })
  hl("MatchParen", { bg = c.selection, bold = true })
  hl("Search", { fg = c.fg, bg = c.warn_bg, sp = c.warn_border, underline = true })
  hl("CurSearch", { fg = c.accent_fg, bg = c.accent })
  hl("IncSearch", { link = "CurSearch" })
  hl("Substitute", { link = "CurSearch" })
  hl("WinSeparator", { fg = c.border })
  hl("VertSplit", { link = "WinSeparator" })
  hl("StatusLine", { fg = c.fg, bg = c.hover })
  hl("StatusLineNC", { fg = c.muted, bg = c.code_bg })
  hl("TabLine", { fg = c.muted, bg = c.code_bg })
  hl("TabLineSel", { fg = c.fg, bg = c.bg, bold = true })
  hl("TabLineFill", { bg = c.code_bg })
  hl("WinBar", { fg = c.fg, bold = true })
  hl("Pmenu", { fg = c.fg, bg = c.bg })
  hl("PmenuSel", { fg = c.accent_fg, bg = c.accent })
  hl("PmenuSbar", { bg = c.hover })
  hl("PmenuThumb", { bg = c.border })
  hl("PmenuKind", { fg = c.syn_function, bg = c.bg })
  hl("PmenuExtra", { fg = c.muted, bg = c.bg })
  hl("WildMenu", { link = "PmenuSel" })
  hl("MsgArea", { fg = c.fg, bg = c.code_bg })
  hl("ModeMsg", { fg = c.fg, bold = true })
  hl("MoreMsg", { fg = c.ok })
  hl("Question", { fg = c.ok })
  hl("ErrorMsg", { fg = c.bad })
  hl("WarningMsg", { fg = c.warn_border })
  hl("Title", { fg = c.syn_heading, bold = true })
  hl("Directory", { fg = c.link })
  hl("QuickFixLine", { bg = c.selection })
  hl("DiffAdd", { fg = c.ok })
  hl("DiffDelete", { fg = c.bad })
  hl("DiffChange", { fg = c.warn_border })
  hl("DiffText", { fg = c.warn_border, bold = true })
  hl("SpellBad", { sp = c.bad, undercurl = true })
  hl("SpellCap", { sp = c.warn_border, undercurl = true })
  hl("SpellRare", { sp = c.control, undercurl = true })
  hl("SpellLocal", { sp = c.control, undercurl = true })

  -- Syntax, as theme.ts colors CodeMirror's.
  hl("Comment", { fg = c.syn_comment, italic = true })
  hl("String", { fg = c.syn_string })
  hl("Character", { link = "String" })
  hl("Constant", { fg = c.syn_number })
  hl("Number", { fg = c.syn_number })
  hl("Boolean", { fg = c.syn_number })
  hl("Float", { fg = c.syn_number })
  hl("Identifier", { fg = c.syn_variable })
  hl("Function", { fg = c.syn_function })
  hl("Statement", { fg = c.syn_keyword })
  hl("Keyword", { fg = c.syn_keyword })
  hl("Conditional", { fg = c.syn_keyword })
  hl("Repeat", { fg = c.syn_keyword })
  hl("Label", { fg = c.syn_keyword })
  hl("Operator", { fg = c.syn_punct })
  hl("Exception", { fg = c.syn_keyword })
  hl("PreProc", { fg = c.syn_keyword })
  hl("Type", { fg = c.syn_type })
  hl("Special", { fg = c.syn_meta })
  hl("Delimiter", { fg = c.syn_punct })
  hl("Underlined", { fg = c.syn_link, underline = true })
  hl("Todo", { fg = c.syn_keyword, bold = true })
  hl("Error", { fg = c.bad })
  hl("Added", { fg = c.ok })
  hl("Removed", { fg = c.bad })
  hl("Changed", { fg = c.warn_border })

  -- Markdown, legacy syntax and tree-sitter.
  hl("markdownH1", { fg = c.syn_heading, bold = true })
  for i = 2, 6 do
    hl("markdownH" .. i, { link = "markdownH1" })
  end
  hl("markdownHeadingDelimiter", { link = "markdownH1" })
  hl("markdownCode", { fg = c.syn_meta })
  hl("markdownCodeBlock", { fg = c.syn_meta })
  hl("markdownCodeDelimiter", { fg = c.muted })
  hl("markdownUrl", { fg = c.syn_link, underline = true })
  hl("markdownLinkText", { fg = c.syn_link })
  hl("markdownBold", { bold = true })
  hl("markdownItalic", { italic = true })
  hl("markdownBlockquote", { fg = c.syn_comment })
  hl("markdownListMarker", { fg = c.syn_punct })
  hl("@markup.heading", { fg = c.syn_heading, bold = true })
  hl("@markup.strong", { bold = true })
  hl("@markup.italic", { italic = true })
  hl("@markup.strikethrough", { strikethrough = true })
  hl("@markup.link", { fg = c.syn_link })
  hl("@markup.link.url", { fg = c.syn_link, underline = true })
  hl("@markup.raw", { fg = c.syn_meta })
  hl("@markup.quote", { fg = c.syn_comment })
  hl("@markup.list", { fg = c.syn_punct })
  hl("@variable", { fg = c.fg })
  hl("@property", { fg = c.syn_function })
  hl("@punctuation", { fg = c.syn_punct })

  -- Diagnostics and the run buttons.
  hl("DiagnosticError", { fg = c.bad })
  hl("DiagnosticWarn", { fg = c.warn_border })
  hl("DiagnosticInfo", { fg = c.accent })
  hl("DiagnosticHint", { fg = c.muted })
  hl("DiagnosticOk", { fg = c.ok })
  hl("DiagnosticUnderlineError", { sp = c.bad, undercurl = true })
  hl("DiagnosticUnderlineWarn", { sp = c.warn_border, undercurl = true })
  hl("DiagnosticUnderlineInfo", { sp = c.accent, undercurl = true })
  hl("DiagnosticUnderlineHint", { sp = c.muted, undercurl = true })
  hl("mdshRun", { fg = c.accent })

  vim.g.colors_name = "mdsh"
end

-- set_colors uses the app's colors, c, with use, the setting to; without
-- it, the colorscheme from the user's config stays, or Neovim's default,
-- with the theme's background.
local colors
function calls.set_colors(c, use)
  colors = use and c or nil
  if use then
    app_colors(c)
    return
  end
  if vim.g.colors_name == "mdsh" then
    vim.cmd("colorscheme default")
  end
  local bg = c.dark and "dark" or "light"
  if vim.o.background ~= bg then
    vim.o.background = bg
  end
end

-- The user's config runs once the UI attaches, which may be after the
-- app's colors came; a colorscheme it sets doesn't win over the setting.
vim.api.nvim_create_autocmd("VimEnter", {
  group = group,
  once = true,
  callback = function()
    if colors and vim.g.colors_name ~= "mdsh" then
      app_colors(colors)
    end
  end,
})
