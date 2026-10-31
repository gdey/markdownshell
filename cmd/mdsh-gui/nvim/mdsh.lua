-- mdsh-gui's side of the embedded Neovim: the buffer holding the file,
-- kept in step with mdsh-gui over RPC. Run once with nvim_exec_lua, with
-- the channel to mdsh-gui, the buffer's name and its lines.

local chan, name, lines = ...

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
