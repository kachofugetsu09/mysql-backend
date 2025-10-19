package agent

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const defaultModel = "deepseek-chat"

var (
	once      sync.Once
	chatModel model.ChatModel
	initErr   error
)

func initAgent(ctx context.Context) (model.ChatModel, error) {
	once.Do(func() {
		log.Print("[initAgent] start")
		chatModel, initErr = createChatModel(ctx)
		if initErr != nil {
			log.Printf("[initAgent] createChatModel failed: %v", initErr)
			return
		}

		log.Print("[initAgent] ensuring tools")
		_, initErr = ensureTools(ctx)
		if initErr != nil {
			log.Printf("[initAgent] ensureTools failed: %v", initErr)
		}
	})
	if initErr != nil {
		return nil, initErr
	}
	return chatModel, nil
}

func createChatModel(ctx context.Context) (model.ChatModel, error) {
	apiKey := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if apiKey == "" {
		log.Print("[createChatModel] missing api key")
		return nil, fmt.Errorf("DEEPSEEK_API_KEY 未设置")
	}

	modelID := strings.TrimSpace(os.Getenv("DEEPSEEK_MODEL"))
	if modelID == "" {
		modelID = defaultModel
	}

	cfg := &deepseek.ChatModelConfig{
		APIKey: apiKey,
		Model:  modelID,
	}
	if base := strings.TrimSpace(os.Getenv("DEEPSEEK_BASE_URL")); base != "" {
		cfg.BaseURL = base
	}

	chat, err := deepseek.NewChatModel(ctx, cfg)
	if err != nil {
		log.Printf("[createChatModel] NewChatModel error: %v", err)
		return nil, fmt.Errorf("创建 deepseek 模型失败: %w", err)
	}
	log.Print("[createChatModel] success")
	return chat, nil
}

func Generate(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("消息不能为空")
	}

	chat, err := initAgent(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := chat.Generate(ctx, messages)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func ChatModel(ctx context.Context) (model.ChatModel, error) {
	return initAgent(ctx)
}

func Tools(ctx context.Context) ([]tool.InvokableTool, error) {
	if _, err := initAgent(ctx); err != nil {
		return nil, err
	}
	return RegisteredTools(ctx)
}
