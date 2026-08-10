package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"bruncher-ai/agent"
	"bruncher-ai/tools"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)
type responseMsg struct {
	text string
	err  error
}

type chatLog struct {
	sender string // "user", "agent", or "status"
	text   string
}

type model struct {
	viewport   viewport.Model
	textarea   textarea.Model
	llm        *agent.LLMClient
	executor   *tools.ToolExecutor
	history    []map[string]interface{}
	chatLogs   []chatLog
	loading    bool
	width      int
	height     int
}

func initialModel() model {
	ta := textarea.New()
	ta.Placeholder = "Type a task (e.g. 'open calculator') and press Enter..."
	ta.Focus()
	ta.CharLimit = 500
	ta.SetWidth(80)
	ta.SetHeight(3)

	vp := viewport.New(80, 15)

systemPrompt := `You are bruncher-AI, an open-source Windows 11 AI desktop assistant created by creator-bruncher on GitHub.
If anyone asks who created you, who made you, or about your origins, state clearly that you are bruncher-AI, created by creator-bruncher and available as an open-source project on GitHub.

If asked to perform a desktop task, open an application, or run commands, output ONLY a JSON object:
{"action": "powershell", "command": "<powershell script>"}

Windows Executable Rules:
- Always use precise Windows executable names or URI schemes:
  * Calculator -> Start-Process calc (or Start-Process calculator:)
  * Notepad -> Start-Process notepad
  * Paint -> Start-Process mspaint
  * Settings -> Start-Process ms-settings:

If chatting, reply in plain text. Keep responses concise.`

	m := model{
		textarea: ta,
		viewport: vp,
		llm:      agent.NewLocalLLMClient("qwen2.5-coder:3b"),
		executor: tools.NewToolExecutor(),
		history: []map[string]interface{}{
			{"role": "system", "content": systemPrompt},
		},
		chatLogs: []chatLog{
			{sender: "status", text: "🤖 bruncher-AI Active (Full Windows 11 Control Mode)\nCreated by @creator-bruncher on GitHub\n--------------------------------------------------"},
		},
		width:  80,
		height: 20,
	}

	m.renderViewport()
	return m
}

func (m *model) renderViewport() {
	var fullContent strings.Builder
	width := m.viewport.Width - 4
	if width <= 0 {
		width = 76
	}

	for _, log := range m.chatLogs {
		var prefix string
		switch log.sender {
		case "user":
			prefix = "👤 User: "
		case "agent":
			prefix = "🤖 Agent: "
		default:
			prefix = ""
		}

		wrapped := wordwrap.String(prefix+log.text, width)
		fullContent.WriteString(wrapped + "\n\n")
	}

	if m.loading {
		fullContent.WriteString("⏳ Thinking...\n")
	}

	m.viewport.SetContent(fullContent.String())
	m.viewport.GotoBottom()
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func sendLLMQuery(llm *agent.LLMClient, executor *tools.ToolExecutor, history []map[string]interface{}) tea.Cmd {
	return func() tea.Msg {
		res, err := llm.Query(history, nil)
		if err != nil {
			return responseMsg{err: err}
		}

		choices, ok := res["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			return responseMsg{text: "No response received from local LLM."}
		}

		choice := choices[0].(map[string]interface{})
		message := choice["message"].(map[string]interface{})
		content, _ := message["content"].(string)

		content = strings.TrimSpace(content)
		if strings.HasPrefix(content, "```json") {
			content = strings.TrimPrefix(content, "```json")
			content = strings.TrimSuffix(content, "```")
			content = strings.TrimSpace(content)
		} else if strings.HasPrefix(content, "```") {
			content = strings.TrimPrefix(content, "```")
			content = strings.TrimSuffix(content, "```")
			content = strings.TrimSpace(content)
		}

		var toolCall struct {
			Action  string                 `json:"action"`
			Command string                 `json:"command"`
			Params  map[string]interface{} `json:"params"`
			Text    string                 `json:"text"`
			Button  string                 `json:"button"`
		}

		if err := json.Unmarshal([]byte(content), &toolCall); err == nil && toolCall.Action != "" {
			params := make(map[string]interface{})
			if toolCall.Command != "" {
				params["command"] = toolCall.Command
			}
			if toolCall.Text != "" {
				params["text"] = toolCall.Text
			}
			if toolCall.Button != "" {
				params["button"] = toolCall.Button
			}
			for k, v := range toolCall.Params {
				params[k] = v
			}

			output, execErr := executor.Execute(toolCall.Action, params)
			if execErr != nil {
				return responseMsg{text: fmt.Sprintf("⚠️ Failed to execute %s: %v", toolCall.Action, execErr)}
			}
			if output == "" {
				output = "Done."
			}
			return responseMsg{text: fmt.Sprintf("⚙️ Executed [%s]: %s", toolCall.Action, output)}
		}

		return responseMsg{text: content}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = msg.Height - 8
		m.textarea.SetWidth(msg.Width - 4)
		m.renderViewport()

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			if m.loading {
				return m, nil
			}

			userText := strings.TrimSpace(m.textarea.Value())
			if userText == "" {
				return m, nil
			}

			m.textarea.Reset()
			m.chatLogs = append(m.chatLogs, chatLog{sender: "user", text: userText})
			m.history = append(m.history, map[string]interface{}{
				"role":    "user",
				"content": userText,
			})

			m.loading = true
			m.renderViewport()

			return m, sendLLMQuery(m.llm, m.executor, m.history)
		}

	case responseMsg:
		m.loading = false
		if msg.err != nil {
			m.chatLogs = append(m.chatLogs, chatLog{sender: "status", text: fmt.Sprintf("❌ Error: %v", msg.err)})
		} else {
			m.chatLogs = append(m.chatLogs, chatLog{sender: "agent", text: msg.text})
			m.history = append(m.history, map[string]interface{}{
				"role":    "assistant",
				"content": msg.text,
			})
		}

		m.renderViewport()
		return m, nil
	}

	m.textarea, tiCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)

	return m, tea.Batch(tiCmd, vpCmd)
}

func (m model) View() string {
	style := lipgloss.NewStyle().Margin(1, 1)
	return style.Render(
		fmt.Sprintf("%s\n\n%s", m.viewport.View(), m.textarea.View()),
	)
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}