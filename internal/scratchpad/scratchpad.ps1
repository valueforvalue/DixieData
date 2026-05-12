param(
    [Parameter(Mandatory = $true)][string]$DisplayId,
    [Parameter(Mandatory = $true)][string]$TextPath,
    [Parameter(Mandatory = $true)][string]$StatePath
)

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Runtime.InteropServices;

public static class ScratchpadNative {
    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    public static extern IntPtr FindWindow(string lpClassName, string lpWindowName);

    [DllImport("user32.dll")]
    public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);

    [DllImport("user32.dll")]
    public static extern bool SetForegroundWindow(IntPtr hWnd);
}
"@

$safeDisplayId = ($DisplayId -replace '[^A-Za-z0-9_-]', '-').Trim('-')
if ([string]::IsNullOrWhiteSpace($safeDisplayId)) {
    $safeDisplayId = 'unfiled'
}

$windowTitle = "DixieData Scratch Pad - $DisplayId"
$mutexCreated = $false
$mutex = New-Object System.Threading.Mutex($true, "Local\DixieDataScratchPad_$safeDisplayId", [ref]$mutexCreated)

if (-not $mutexCreated) {
    $existing = [ScratchpadNative]::FindWindow($null, $windowTitle)
    if ($existing -ne [IntPtr]::Zero) {
        [ScratchpadNative]::ShowWindow($existing, 9) | Out-Null
        [ScratchpadNative]::SetForegroundWindow($existing) | Out-Null
    }
    exit 0
}

try {
    $textDirectory = [System.IO.Path]::GetDirectoryName($TextPath)
    if (-not [string]::IsNullOrWhiteSpace($textDirectory)) {
        [System.IO.Directory]::CreateDirectory($textDirectory) | Out-Null
    }

    if (-not [System.IO.File]::Exists($TextPath)) {
        [System.IO.File]::WriteAllText($TextPath, '', [System.Text.Encoding]::UTF8)
    }

    function Get-DesktopBounds {
        $screens = [System.Windows.Forms.Screen]::AllScreens
        if ($screens.Length -eq 0) {
            return @{ Left = 0; Top = 0; Right = 1280; Bottom = 720 }
        }
        $left = $screens[0].WorkingArea.Left
        $top = $screens[0].WorkingArea.Top
        $right = $screens[0].WorkingArea.Right
        $bottom = $screens[0].WorkingArea.Bottom
        foreach ($screen in $screens) {
            $left = [Math]::Min($left, $screen.WorkingArea.Left)
            $top = [Math]::Min($top, $screen.WorkingArea.Top)
            $right = [Math]::Max($right, $screen.WorkingArea.Right)
            $bottom = [Math]::Max($bottom, $screen.WorkingArea.Bottom)
        }
        return @{ Left = $left; Top = $top; Right = $right; Bottom = $bottom }
    }

    function Clamp-WindowRect([int]$X, [int]$Y, [int]$Width, [int]$Height, [System.Drawing.Size]$MinimumSize) {
        $bounds = Get-DesktopBounds
        $width = [Math]::Max($MinimumSize.Width, $Width)
        $height = [Math]::Max($MinimumSize.Height, $Height)
        $maxWidth = [Math]::Max($MinimumSize.Width, $bounds.Right - $bounds.Left - 24)
        $maxHeight = [Math]::Max($MinimumSize.Height, $bounds.Bottom - $bounds.Top - 24)
        $width = [Math]::Min($width, $maxWidth)
        $height = [Math]::Min($height, $maxHeight)
        $maxX = [Math]::Max($bounds.Left + 12, $bounds.Right - $width - 12)
        $maxY = [Math]::Max($bounds.Top + 12, $bounds.Bottom - $height - 12)
        $x = [Math]::Min([Math]::Max($X, $bounds.Left + 12), $maxX)
        $y = [Math]::Min([Math]::Max($Y, $bounds.Top + 12), $maxY)
        return @{ X = $x; Y = $y; Width = $width; Height = $height }
    }

    function Load-State {
        if (-not [System.IO.File]::Exists($StatePath)) {
            return $null
        }
        try {
            $raw = [System.IO.File]::ReadAllText($StatePath, [System.Text.Encoding]::UTF8)
            if ([string]::IsNullOrWhiteSpace($raw)) {
                return $null
            }
            return $raw | ConvertFrom-Json
        } catch {
            return $null
        }
    }

    $savingState = $false
    function Save-State([System.Windows.Forms.Form]$Form) {
        if ($savingState -or $Form.WindowState -eq [System.Windows.Forms.FormWindowState]::Minimized) {
            return
        }
        $savingState = $true
        try {
            $payload = @{
                x = $Form.Left
                y = $Form.Top
                width = $Form.Width
                height = $Form.Height
                pinned = $Form.TopMost
            } | ConvertTo-Json -Compress
            [System.IO.File]::WriteAllText($StatePath, $payload, [System.Text.Encoding]::UTF8)
        } finally {
            $savingState = $false
        }
    }

    $minimumSize = New-Object System.Drawing.Size(360, 260)
    $state = Load-State
    $desiredX = if ($null -ne $state -and $state.PSObject.Properties.Match('x').Count -gt 0) { [int]$state.x } else { 120 }
    $desiredY = if ($null -ne $state -and $state.PSObject.Properties.Match('y').Count -gt 0) { [int]$state.y } else { 120 }
    $desiredWidth = if ($null -ne $state -and $state.PSObject.Properties.Match('width').Count -gt 0) { [int]$state.width } else { 520 }
    $desiredHeight = if ($null -ne $state -and $state.PSObject.Properties.Match('height').Count -gt 0) { [int]$state.height } else { 380 }
    $rect = Clamp-WindowRect $desiredX $desiredY $desiredWidth $desiredHeight $minimumSize
    $pinned = $true
    if ($null -ne $state -and $state.PSObject.Properties.Match('pinned').Count -gt 0) {
        $pinned = [bool]$state.pinned
    }

    $form = New-Object System.Windows.Forms.Form
    $form.Text = $windowTitle
    $form.StartPosition = [System.Windows.Forms.FormStartPosition]::Manual
    $form.MinimumSize = $minimumSize
    $form.TopMost = $pinned
    $form.Location = New-Object System.Drawing.Point($rect.X, $rect.Y)
    $form.Size = New-Object System.Drawing.Size($rect.Width, $rect.Height)
    $form.BackColor = [System.Drawing.Color]::FromArgb(246, 241, 228)

    $header = New-Object System.Windows.Forms.FlowLayoutPanel
    $header.Dock = [System.Windows.Forms.DockStyle]::Top
    $header.FlowDirection = [System.Windows.Forms.FlowDirection]::RightToLeft
    $header.WrapContents = $false
    $header.AutoSize = $false
    $header.Height = 42
    $header.Padding = New-Object System.Windows.Forms.Padding(8, 8, 8, 4)
    $header.BackColor = [System.Drawing.Color]::FromArgb(233, 225, 203)

    $pinButton = New-Object System.Windows.Forms.Button
    $pinButton.AutoSize = $true
    $pinButton.Text = if ($form.TopMost) { 'Pinned' } else { 'Pin' }

    $copyButton = New-Object System.Windows.Forms.Button
    $copyButton.AutoSize = $true
    $copyButton.Text = 'Copy All'

    $clearButton = New-Object System.Windows.Forms.Button
    $clearButton.AutoSize = $true
    $clearButton.Text = 'Clear'

    $header.Controls.AddRange(@($pinButton, $copyButton, $clearButton))

    $summary = New-Object System.Windows.Forms.Label
    $summary.Dock = [System.Windows.Forms.DockStyle]::Top
    $summary.AutoSize = $false
    $summary.Height = 34
    $summary.Padding = New-Object System.Windows.Forms.Padding(12, 8, 12, 0)
    $summary.Text = "Record ID: $DisplayId"

    $textbox = New-Object System.Windows.Forms.TextBox
    $textbox.Multiline = $true
    $textbox.AcceptsReturn = $true
    $textbox.AcceptsTab = $true
    $textbox.ScrollBars = [System.Windows.Forms.ScrollBars]::Both
    $textbox.WordWrap = $false
    $textbox.Dock = [System.Windows.Forms.DockStyle]::Fill
    $textbox.Font = New-Object System.Drawing.Font('Consolas', 10)
    $textbox.Text = [System.IO.File]::ReadAllText($TextPath, [System.Text.Encoding]::UTF8)

    $contextMenu = New-Object System.Windows.Forms.ContextMenuStrip
    $cutItem = $contextMenu.Items.Add('Cut')
    $copyItem = $contextMenu.Items.Add('Copy')
    $pasteItem = $contextMenu.Items.Add('Paste')
    $selectAllItem = $contextMenu.Items.Add('Select All')
    $textbox.ContextMenuStrip = $contextMenu

    $footer = New-Object System.Windows.Forms.Label
    $footer.Dock = [System.Windows.Forms.DockStyle]::Bottom
    $footer.AutoSize = $false
    $footer.Height = 26
    $footer.Padding = New-Object System.Windows.Forms.Padding(12, 4, 12, 0)
    $footer.Text = 'Local only. Stored per Record ID outside the database.'

    $form.Controls.Add($textbox)
    $form.Controls.Add($footer)
    $form.Controls.Add($summary)
    $form.Controls.Add($header)

    $textbox.Add_TextChanged({
        [System.IO.File]::WriteAllText($TextPath, $textbox.Text, [System.Text.Encoding]::UTF8)
    })

    $pinButton.Add_Click({
        $form.TopMost = -not $form.TopMost
        $pinButton.Text = if ($form.TopMost) { 'Pinned' } else { 'Pin' }
        Save-State $form
    })

    $copyButton.Add_Click({
        if (-not [string]::IsNullOrEmpty($textbox.Text)) {
            [System.Windows.Forms.Clipboard]::SetText($textbox.Text)
        }
        $textbox.Focus()
    })

    $clearButton.Add_Click({
        $textbox.Clear()
        $textbox.Focus()
    })

    $cutItem.Add_Click({ $textbox.Cut() })
    $copyItem.Add_Click({ $textbox.Copy() })
    $pasteItem.Add_Click({ $textbox.Paste() })
    $selectAllItem.Add_Click({
        $textbox.SelectAll()
        $textbox.Focus()
    })

    $contextMenu.Add_Opening({
        $hasSelection = $textbox.SelectionLength -gt 0
        $hasText = -not [string]::IsNullOrEmpty($textbox.Text)
        $cutItem.Enabled = $hasSelection
        $copyItem.Enabled = $hasSelection
        $pasteItem.Enabled = [System.Windows.Forms.Clipboard]::ContainsText()
        $selectAllItem.Enabled = $hasText
    })

    $form.Add_Move({ Save-State $form })
    $form.Add_ResizeEnd({ Save-State $form })
    $form.Add_SizeChanged({
        if ($form.WindowState -eq [System.Windows.Forms.FormWindowState]::Normal) {
            Save-State $form
        }
    })
    $form.Add_FormClosing({ Save-State $form })
    $form.Add_Shown({ $textbox.Focus() })

    [System.Windows.Forms.Application]::EnableVisualStyles()
    [void]$form.ShowDialog()
}
finally {
    if ($null -ne $mutex) {
        $mutex.ReleaseMutex() | Out-Null
        $mutex.Dispose()
    }
}
