package anthropic

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/run-bigpig/jcp/internal/logger"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

var convertLog = logger.New("anthropic:convert")

// toToolResultContent 将函数返回值转换为 Anthropic tool_result.content。
// Anthropic 要求 content 为字符串或内容块数组，这里统一归一为字符串 JSON。
func toToolResultContent(resp any) (json.RawMessage, error) {
	if s, ok := resp.(string); ok {
		b, err := json.Marshal(s)
		return b, err
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(b))
}

// extractTextFromContent 提取 genai.Content 中的纯文本
func extractTextFromContent(content *genai.Content) string {
	if content == nil {
		return ""
	}
	var texts []string
	for _, part := range content.Parts {
		if part.Text != "" && !part.Thought {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// toAnthropicRequest 将 ADK LLMRequest 转换为 Anthropic Messages 请求
func toAnthropicRequest(req *model.LLMRequest, modelName string, noSystemRole bool) (*MessagesRequest, error) {
	ar := &MessagesRequest{
		Model:     modelName,
		MaxTokens: 4096, // Anthropic 要求必须设置
	}

	// 提取系统指令文本
	var systemText string
	if req.Config != nil && req.Config.SystemInstruction != nil {
		systemText = extractTextFromContent(req.Config.SystemInstruction)
	}

	// 转换消息
	msgs, err := toAnthropicMessages(req.Contents)
	if err != nil {
		return nil, err
	}

	// 非官方 API 或不支持 system role：降级为第一条 user message
	if systemText != "" {
		if !noSystemRole {
			ar.System = systemText
		} else {
			systemMsg := Message{
				Role:    "user",
				Content: []ContentBlock{{Type: "text", Text: systemText}},
			}
			// 如果第一条也是 user，合并避免连续 user
			if len(msgs) > 0 && msgs[0].Role == "user" {
				msgs[0].Content = append([]ContentBlock{{Type: "text", Text: systemText}}, msgs[0].Content...)
			} else {
				msgs = append([]Message{systemMsg}, msgs...)
			}
		}
	}

	ar.Messages = msgs

	// 转换工具
	if req.Config != nil && len(req.Config.Tools) > 0 {
		tools, err := convertTools(req.Config.Tools)
		if err != nil {
			return nil, err
		}
		ar.Tools = tools
	}

	// 应用配置
	if req.Config != nil {
		// claude-opus-4 系列不支持 temperature 参数
		// 只有非 opus-4 模型才设置 temperature
		if req.Config.Temperature != nil && !isOpus4Model(modelName) {
			t := float64(*req.Config.Temperature)
			ar.Temperature = &t
		}
		if req.Config.MaxOutputTokens > 0 {
			ar.MaxTokens = int(req.Config.MaxOutputTokens)
		}
		if req.Config.TopP != nil && !isOpus4Model(modelName) {
			p := float64(*req.Config.TopP)
			ar.TopP = &p
		}
		if len(req.Config.StopSequences) > 0 {
			ar.StopSequences = req.Config.StopSequences
		}
	}

	return ar, nil
}

// isOpus4Model 检查是否为 Claude Opus 4 系列模型（不支持 temperature/top_p）
func isOpus4Model(modelName string) bool {
	return strings.Contains(strings.ToLower(modelName), "opus-4") ||
		strings.Contains(strings.ToLower(modelName), "opus4")
}

// toAnthropicMessages 将 genai.Content 列表转换为 Anthropic messages
func toAnthropicMessages(contents []*genai.Content) ([]Message, error) {
	var msgs []Message

	for _, content := range contents {
		if content == nil {
			continue
		}

		role := "user"
		if content.Role == "model" {
			role = "assistant"
		}

		var blocks []ContentBlock

		for _, part := range content.Parts {
			// 跳过 thought parts（不回传给 API）
			if part.Thought {
				continue
			}

			// 文本
			if part.Text != "" {
				blocks = append(blocks, ContentBlock{
					Type: "text",
					Text: part.Text,
				})
			}

			// 函数调用 → tool_use
			if part.FunctionCall != nil {
				inputJSON, err := json.Marshal(part.FunctionCall.Args)
				if err != nil {
					return nil, fmt.Errorf("marshal function call args: %w", err)
				}
				blocks = append(blocks, ContentBlock{
					Type:  "tool_use",
					ID:    part.FunctionCall.ID,
					Name:  part.FunctionCall.Name,
					Input: inputJSON,
				})
			}

			// 函数响应 → tool_result
			if part.FunctionResponse != nil {
				contentJSON, err := toToolResultContent(part.FunctionResponse.Response)
				if err != nil {
					return nil, fmt.Errorf("marshal function response: %w", err)
				}
				blocks = append(blocks, ContentBlock{
					Type:       "tool_result",
					ToolUseID:  part.FunctionResponse.ID,
					RawContent: contentJSON,
				})
			}
		}

		if len(blocks) == 0 {
			continue
		}

		// Anthropic 要求 user/assistant 交替，合并相同 role
		if len(msgs) > 0 && msgs[len(msgs)-1].Role == role {
			msgs[len(msgs)-1].Content = append(msgs[len(msgs)-1].Content, blocks...)
		} else {
			msgs = append(msgs, Message{Role: role, Content: blocks})
		}
	}

	return msgs, nil
}

// convertTools 将 genai.Tool 转换为 Anthropic Tool
func convertTools(genaiTools []*genai.Tool) ([]Tool, error) {
	var tools []Tool
	for _, gt := range genaiTools {
		if gt == nil {
			continue
		}
		for _, fd := range gt.FunctionDeclarations {
			schema := fd.ParametersJsonSchema
			if schema == nil {
				schema = fd.Parameters
			}
			if schema == nil {
				return nil, fmt.Errorf("parameters is nil for tool %s", fd.Name)
			}
			schemaJSON, err := json.Marshal(schema)
			if err != nil {
				return nil, fmt.Errorf("marshal tool schema: %w", err)
			}
			// 清洗 MCP 透传的 JSON Schema，移除 Anthropic 不支持的关键字
			schemaJSON, err = sanitizeSchemaForAnthropic(schemaJSON)
			if err != nil {
				convertLog.Warn("清洗 tool schema 失败 (%s): %v", fd.Name, err)
			}
			tools = append(tools, Tool{
				Name:        fd.Name,
				Description: fd.Description,
				InputSchema: schemaJSON,
			})
		}
	}
	return tools, nil
}

// sanitizeSchemaForAnthropic 清洗 JSON Schema，移除 Anthropic 不支持的关键字
func sanitizeSchemaForAnthropic(raw json.RawMessage) (json.RawMessage, error) {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return raw, err
	}
	sanitizeSchemaNode(schema)
	return json.Marshal(schema)
}

// sanitizeSchemaNode 递归清洗 schema 节点
func sanitizeSchemaNode(node map[string]any) {
	// const → enum 单值
	if val, ok := node["const"]; ok {
		node["enum"] = []any{val}
		delete(node, "const")
	}

	// 移除 default: null
	if v, ok := node["default"]; ok && v == nil {
		delete(node, "default")
	}

	// anyOf 含 {"type":"null"} → 提取非 null 分支，标记非必填
	if anyOf, ok := node["anyOf"].([]any); ok {
		var nonNull []any
		for _, item := range anyOf {
			if m, ok := item.(map[string]any); ok {
				if m["type"] == "null" {
					continue
				}
				nonNull = append(nonNull, m)
			}
		}
		if len(nonNull) == 1 {
			// 单个非 null 分支，展平到当前节点
			if m, ok := nonNull[0].(map[string]any); ok {
				delete(node, "anyOf")
				for k, v := range m {
					node[k] = v
				}
			}
		} else if len(nonNull) > 1 {
			node["anyOf"] = nonNull
		}
	}

	// 递归处理 properties
	if props, ok := node["properties"].(map[string]any); ok {
		for _, v := range props {
			if m, ok := v.(map[string]any); ok {
				sanitizeSchemaNode(m)
			}
		}
	}

	// 递归处理 items
	if items, ok := node["items"].(map[string]any); ok {
		sanitizeSchemaNode(items)
	}
}

// convertAnthropicResponse 将 Anthropic 响应转换为 ADK LLMResponse
func convertAnthropicResponse(resp *MessagesResponse) (*model.LLMResponse, error) {
	content := &genai.Content{
		Role:  genai.RoleModel,
		Parts: []*genai.Part{},
	}

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				content.Parts = append(content.Parts, &genai.Part{Text: block.Text})
			}
		case "thinking":
			if block.Thinking != "" {
				content.Parts = append(content.Parts, &genai.Part{Text: block.Thinking, Thought: true})
			}
		case "tool_use":
			args := make(map[string]any)
			if len(block.Input) > 0 {
				if err := json.Unmarshal(block.Input, &args); err != nil {
					convertLog.Warn("解析 tool_use input 失败: %v", err)
				}
			}
			content.Parts = append(content.Parts, &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   block.ID,
					Name: block.Name,
					Args: args,
				},
			})
		}
	}

	return &model.LLMResponse{
		Content:       content,
		UsageMetadata: convertUsage(&resp.Usage),
		FinishReason:  convertStopReason(resp.StopReason),
		TurnComplete:  true,
	}, nil
}

// convertUsage 转换 token 用量
func convertUsage(u *Usage) *genai.GenerateContentResponseUsageMetadata {
	if u == nil {
		return nil
	}
	return &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:     int32(u.InputTokens),
		CandidatesTokenCount: int32(u.OutputTokens),
		TotalTokenCount:      int32(u.InputTokens + u.OutputTokens),
	}
}

// convertStopReason 转换停止原因
func convertStopReason(reason string) genai.FinishReason {
	switch reason {
	case "end_turn", "stop_sequence":
		return genai.FinishReasonStop
	case "max_tokens":
		return genai.FinishReasonMaxTokens
	case "tool_use":
		return genai.FinishReasonStop
	default:
		return genai.FinishReasonUnspecified
	}
}
