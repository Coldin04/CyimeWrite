package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"g.co1d.in/Coldin04/Cyime/server/internal/config"
	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"g.co1d.in/Coldin04/Cyime/server/internal/securevalue"
	"g.co1d.in/Coldin04/Cyime/server/internal/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProviderTemplate represents a predefined OAuth/OIDC provider configuration
type ProviderTemplate struct {
	Name        string
	DisplayName string
	IssuerURL   string
	AuthURL     string
	TokenURL    string
	UserInfoURL string
	Scopes      string
	IconURL     string
}

// Helper function to convert string to pointer
func strPtr(s string) *string {
	return &s
}

// Common provider templates
var providerTemplates = map[string]ProviderTemplate{
	"github": {
		Name:        "github",
		DisplayName: "GitHub",
		AuthURL:     "https://github.com/login/oauth/authorize",
		TokenURL:    "https://github.com/login/oauth/access_token",
		UserInfoURL: "https://api.github.com/user",
		Scopes:      "read:user user:email",
		IconURL:     "https://github.com/fluidicon.png",
	},
	"google": {
		Name:        "google",
		DisplayName: "Google",
		IssuerURL:   "https://accounts.google.com",
		AuthURL:     "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:    "https://oauth2.googleapis.com/token",
		UserInfoURL: "https://www.googleapis.com/oauth2/v2/userinfo",
		Scopes:      "openid email profile",
		IconURL:     "https://lh3.googleusercontent.com/COxitqgJr1sJnIDe8-jiKhxDx1FrYbtRHKJ9z_hELisAlapwE9LUPh6fcXIfb5-twpw",
	},
}

func main() {
	err := config.LoadDotEnv(".env")
	if err != nil {
		switch {
		case strings.ToLower(os.Getenv("ENV")) == "development":
			// create an .env before to ensure the next step work fine.:<
			// get env from root of this project before, if not exist, generate a minimal one.
			log.Println(".env 没有找到，正在从 .env.example 中复制配置")
			runtimeCallerPath := utils.GetRunTimeCallerPath()
			envExamplePath := filepath.Join(runtimeCallerPath, "../../", ".env.example")
			envPath := filepath.Join(runtimeCallerPath, ".env")
			envExampleFile, err := os.ReadFile(envExamplePath)
			if err != nil {
				log.Fatalf("读取 .env.example 失败: %v, 请通过原始 ENV 获取一份 ENV 样本后重试流程!", err)
			}
			err = os.WriteFile(envPath, envExampleFile, 0644)
			if err != nil {
				log.Fatalf("创建 .env 失败: %v", err)
				return
			}
			// replace the JWT_SECRET_KEY with a random SHA256 hash
			jwtSecretKey := utils.GenerateRandomSHA256()
			appEncryptionKey := utils.GenerateRandomSHA256()
			envExampleFile = bytes.ReplaceAll(envExampleFile, []byte("JWT_SECRET_KEY=replace-with-a-strong-secret"), []byte("JWT_SECRET_KEY="+jwtSecretKey))
			envExampleFile = bytes.ReplaceAll(envExampleFile, []byte("APP_ENCRYPTION_KEY=replace-with-a-strong-secret"), []byte("APP_ENCRYPTION_KEY="+appEncryptionKey))
			envExampleFile = bytes.ReplaceAll(envExampleFile, []byte("\nAPP_ENCRYPTION_KEY=\n"), []byte("\nAPP_ENCRYPTION_KEY="+appEncryptionKey+"\n"))
			err = os.WriteFile(envPath, envExampleFile, 0644)
			if err != nil {
				log.Fatalf("替换 JWT_SECRET_KEY 失败: %v", err)
				return
			}
			fmt.Println("已创建 .env 文件")
			fmt.Println("请修改 .env 文件中的配置，然后重新运行初始化工具")
			return
		default:
			log.Fatalf("加载 .env 失败: %v, 请检查 .env 文件是否存在且正确配置", err)
			return
		}
	}

	fmt.Println("🍋 Cyime 初始化向导")
	fmt.Println("========================")
	fmt.Println()

	// Initialize database
	database.Connect()
	log.Println("数据库连接成功")

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("请选择 OAuth/SSO 登录提供商管理操作：")
	fmt.Println("1. 新增登录提供商")
	fmt.Println("2. 列出当前登录提供商")
	fmt.Println("3. 删除单个登录提供商")
	fmt.Println("4. 清空全部登录提供商")
	fmt.Println("5. 修改提供商显示名称")
	fmt.Println("6. 跳过（退出）")
	fmt.Print("请选择 (1-6): ")

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch choice {
	case "1":
		handleCreateProvider(reader)
	case "2":
		if err := listProviders(); err != nil {
			log.Fatalf("列出提供商失败：%v", err)
		}
	case "3":
		if err := deleteProviderInteractive(reader); err != nil {
			log.Fatalf("删除提供商失败：%v", err)
		}
	case "4":
		if err := clearProvidersInteractive(reader); err != nil {
			log.Fatalf("清空提供商失败：%v", err)
		}
	case "5":
		if err := updateProviderDisplayNameInteractive(reader); err != nil {
			log.Fatalf("修改提供商显示名称失败：%v", err)
		}
	case "6":
		fmt.Println("已退出。你可以稍后重新运行初始化向导。")
	default:
		fmt.Println("无效的选择，已退出。")
	}
}

func handleCreateProvider(reader *bufio.Reader) {
	fmt.Println()
	fmt.Println("请选择要新增的登录提供商类型：")
	fmt.Println("1. GitHub")
	fmt.Println("2. Google")
	fmt.Println("3. 自定义 OIDC 提供商")
	fmt.Println("4. 取消")
	fmt.Print("请选择 (1-4): ")

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	if choice == "4" {
		fmt.Println("已取消新增提供商。")
		return
	}

	var provider models.AuthProvider

	switch choice {
	case "1":
		provider = configureGitHubProvider(reader)
	case "2":
		provider = configureGoogleProvider(reader)
	case "3":
		provider = configureCustomProvider(reader)
	default:
		fmt.Println("无效的选择，已取消新增提供商。")
		return
	}

	if !strings.HasPrefix(provider.ClientSecretEncrypted, securevalue.EncryptedValuePrefix) && provider.ClientSecretEncrypted != "" {
		encryptedSecret, err := securevalue.EncryptString(provider.ClientSecretEncrypted)
		if err != nil {
			log.Fatalf("加密提供商密钥失败：%v", err)
		}
		provider.ClientSecretEncrypted = encryptedSecret
	}

	if err := database.DB.Create(&provider).Error; err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			fmt.Printf("⚠️  提供商 '%s' 已存在，跳过创建。\n", provider.Name)
		} else {
			log.Fatalf("保存提供商配置失败：%v", err)
		}
		return
	}

	fmt.Printf("✅ 成功配置 %s 登录提供商！\n", provider.Name)
	fmt.Println()
	fmt.Println("下一步:")
	fmt.Println("1. 启动服务器：go run cmd/server/main.go")
	fmt.Println("2. 访问 http://localhost:8080 进行测试")
}

func listProviders() error {
	var providers []models.AuthProvider
	if err := database.DB.Order("name ASC").Find(&providers).Error; err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("当前登录提供商：")
	if len(providers) == 0 {
		fmt.Println("  (无)")
		return nil
	}

	for _, provider := range providers {
		status := "禁用"
		if provider.IsActive {
			status = "启用"
		}
		displayName := provider.Name
		if provider.DisplayName != nil && strings.TrimSpace(*provider.DisplayName) != "" {
			displayName = strings.TrimSpace(*provider.DisplayName)
		}
		fmt.Printf("- %s (%s) [%s] client_id=%s status=%s\n", displayName, provider.Name, provider.ProtocolType, provider.ClientID, status)
	}
	return nil
}

func deleteProviderInteractive(reader *bufio.Reader) error {
	if err := listProviders(); err != nil {
		return err
	}

	fmt.Println()
	fmt.Print("输入要删除的提供商名称: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		fmt.Println("未输入提供商名称，已取消。")
		return nil
	}

	var provider models.AuthProvider
	if err := database.DB.Where("name = ?", name).First(&provider).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			fmt.Printf("未找到提供商 '%s'。\n", name)
			return nil
		}
		return err
	}

	var identityCount int64
	if err := database.DB.Model(&models.UserIdentityProvider{}).Where("provider_name = ?", name).Count(&identityCount).Error; err != nil {
		return err
	}

	fmt.Printf("即将删除提供商 '%s'，并清理 %d 条用户身份映射。输入 yes 确认: ", name, identityCount)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))
	if confirm != "yes" {
		fmt.Println("已取消删除。")
		return nil
	}

	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("provider_name = ?", name).Delete(&models.UserIdentityProvider{}).Error; err != nil {
			return err
		}
		if err := tx.Where("name = ?", name).Delete(&models.AuthProvider{}).Error; err != nil {
			return err
		}
		fmt.Printf("✅ 已删除提供商 '%s'。\n", name)
		return nil
	})
}

func clearProvidersInteractive(reader *bufio.Reader) error {
	var providerCount int64
	if err := database.DB.Model(&models.AuthProvider{}).Count(&providerCount).Error; err != nil {
		return err
	}

	var identityCount int64
	if err := database.DB.Model(&models.UserIdentityProvider{}).Count(&identityCount).Error; err != nil {
		return err
	}

	if providerCount == 0 && identityCount == 0 {
		fmt.Println("当前没有可清理的登录配置。")
		return nil
	}

	fmt.Printf("即将清空 %d 个登录提供商，并删除 %d 条用户身份映射。输入 yes 确认: ", providerCount, identityCount)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))
	if confirm != "yes" {
		fmt.Println("已取消清空。")
		return nil
	}

	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.UserIdentityProvider{}).Error; err != nil {
			return err
		}
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.AuthProvider{}).Error; err != nil {
			return err
		}
		fmt.Println("✅ 已清空全部登录提供商配置。")
		return nil
	})
}

func updateProviderDisplayNameInteractive(reader *bufio.Reader) error {
	if err := listProviders(); err != nil {
		return err
	}

	fmt.Println()
	fmt.Print("输入要修改的提供商 name（括号中的值）: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		fmt.Println("未输入提供商 name，已取消。")
		return nil
	}

	var provider models.AuthProvider
	if err := database.DB.Where("name = ?", name).First(&provider).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			fmt.Printf("未找到提供商 '%s'。\n", name)
			return nil
		}
		return err
	}

	currentDisplayName := provider.Name
	if provider.DisplayName != nil && strings.TrimSpace(*provider.DisplayName) != "" {
		currentDisplayName = strings.TrimSpace(*provider.DisplayName)
	}

	fmt.Printf("当前显示名称: %s\n", currentDisplayName)
	fmt.Print("新的显示名称（留空则清除，回退到 name 显示）: ")
	nextDisplayName, _ := reader.ReadString('\n')
	nextDisplayName = strings.TrimSpace(nextDisplayName)

	updates := map[string]any{
		"display_name": nil,
	}
	if nextDisplayName != "" {
		updates["display_name"] = nextDisplayName
	}

	if err := database.DB.Model(&provider).Updates(updates).Error; err != nil {
		return err
	}

	if nextDisplayName == "" {
		fmt.Printf("✅ 已清除 '%s' 的显示名称，前端将回退显示为 name。\n", provider.Name)
	} else {
		fmt.Printf("✅ 已将 '%s' 的显示名称更新为 '%s'。\n", provider.Name, nextDisplayName)
	}
	return nil
}

func configureGitHubProvider(reader *bufio.Reader) models.AuthProvider {
	fmt.Println()
	fmt.Println("📦 配置 GitHub OAuth")
	fmt.Println("-------------------")
	fmt.Println("请在 GitHub OAuth Apps 页面创建应用：")
	fmt.Println("https://github.com/settings/developers")
	fmt.Println()
	fmt.Println("授权回调 URL 应设置为：http://localhost:8080/api/v1/auth/callback/github")
	fmt.Println()

	fmt.Print("Client ID: ")
	clientID, _ := reader.ReadString('\n')
	clientID = strings.TrimSpace(clientID)

	fmt.Print("Client Secret: ")
	clientSecret, _ := reader.ReadString('\n')
	clientSecret = strings.TrimSpace(clientSecret)

	return models.AuthProvider{
		ID:                    uuid.New(),
		Name:                  "github",
		DisplayName:           strPtr(providerTemplates["github"].DisplayName),
		ProtocolType:          "oauth2",
		AuthURL:               strPtr(providerTemplates["github"].AuthURL),
		TokenURL:              strPtr(providerTemplates["github"].TokenURL),
		UserInfoURL:           strPtr(providerTemplates["github"].UserInfoURL),
		ClientID:              clientID,
		ClientSecretEncrypted: clientSecret,
		IconURL:               strPtr(providerTemplates["github"].IconURL),
		Scopes:                providerTemplates["github"].Scopes,
		IsActive:              true,
	}
}

func configureGoogleProvider(reader *bufio.Reader) models.AuthProvider {
	fmt.Println()
	fmt.Println("📦 配置 Google OAuth")
	fmt.Println("-------------------")
	fmt.Println("请在 Google Cloud Console 创建 OAuth 凭据：")
	fmt.Println("https://console.cloud.google.com/apis/credentials")
	fmt.Println()
	fmt.Println("授权重定向 URI 应设置为：http://localhost:8080/api/v1/auth/callback/google")
	fmt.Println()

	fmt.Print("Client ID: ")
	clientID, _ := reader.ReadString('\n')
	clientID = strings.TrimSpace(clientID)

	fmt.Print("Client Secret: ")
	clientSecret, _ := reader.ReadString('\n')
	clientSecret = strings.TrimSpace(clientSecret)

	return models.AuthProvider{
		ID:                    uuid.New(),
		Name:                  "google",
		DisplayName:           strPtr(providerTemplates["google"].DisplayName),
		ProtocolType:          "oauth2",
		IssuerURL:             strPtr(providerTemplates["google"].IssuerURL),
		AuthURL:               strPtr(providerTemplates["google"].AuthURL),
		TokenURL:              strPtr(providerTemplates["google"].TokenURL),
		UserInfoURL:           strPtr(providerTemplates["google"].UserInfoURL),
		ClientID:              clientID,
		ClientSecretEncrypted: clientSecret,
		IconURL:               strPtr(providerTemplates["google"].IconURL),
		Scopes:                providerTemplates["google"].Scopes,
		IsActive:              true,
	}
}

func configureCustomProvider(reader *bufio.Reader) models.AuthProvider {
	fmt.Println()
	fmt.Println("📦 配置自定义 OIDC 提供商")
	fmt.Println("-----------------------")

	fmt.Print("提供商名称 (例如：keycloak, auth0): ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)

	fmt.Print("显示名称 (可选，例如：微信, GitHub): ")
	displayName, _ := reader.ReadString('\n')
	displayName = strings.TrimSpace(displayName)

	fmt.Print("Issuer URL: ")
	issuerURL, _ := reader.ReadString('\n')
	issuerURL = strings.TrimSpace(issuerURL)

	fmt.Print("Auth URL: ")
	authURL, _ := reader.ReadString('\n')
	authURL = strings.TrimSpace(authURL)

	fmt.Print("Token URL: ")
	tokenURL, _ := reader.ReadString('\n')
	tokenURL = strings.TrimSpace(tokenURL)

	fmt.Print("UserInfo URL: ")
	userInfoURL, _ := reader.ReadString('\n')
	userInfoURL = strings.TrimSpace(userInfoURL)

	fmt.Print("Client ID: ")
	clientID, _ := reader.ReadString('\n')
	clientID = strings.TrimSpace(clientID)

	fmt.Print("Client Secret: ")
	clientSecret, _ := reader.ReadString('\n')
	clientSecret = strings.TrimSpace(clientSecret)

	fmt.Print("Scopes (空格分隔，例如：openid email profile): ")
	scopes, _ := reader.ReadString('\n')
	scopes = strings.TrimSpace(scopes)

	fmt.Print("Icon URL (可选): ")
	iconURL, _ := reader.ReadString('\n')
	iconURL = strings.TrimSpace(iconURL)

	var displayNamePtr *string
	if displayName != "" {
		displayNamePtr = &displayName
	}

	var iconURLPtr *string
	if iconURL != "" {
		iconURLPtr = &iconURL
	}

	return models.AuthProvider{
		ID:                    uuid.New(),
		Name:                  name,
		DisplayName:           displayNamePtr,
		ProtocolType:          "oidc",
		IssuerURL:             &issuerURL,
		AuthURL:               &authURL,
		TokenURL:              &tokenURL,
		UserInfoURL:           &userInfoURL,
		ClientID:              clientID,
		ClientSecretEncrypted: clientSecret,
		IconURL:               iconURLPtr,
		Scopes:                scopes,
		IsActive:              true,
	}
}

// Helper function to check if a provider exists
func providerExists(name string) bool {
	var count int64
	database.DB.Model(&models.AuthProvider{}).Where("name = ?", name).Count(&count)
	return count > 0
}

// Helper function to handle provider update or create
func upsertProvider(provider *models.AuthProvider) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		// Check if provider exists
		var existing models.AuthProvider
		result := tx.Where("name = ?", provider.Name).First(&existing)

		if result.Error == nil {
			// Update existing provider
			return tx.Model(&existing).Updates(provider).Error
		} else if result.Error == gorm.ErrRecordNotFound {
			// Create new provider
			return tx.Create(provider).Error
		}

		return result.Error
	})
}
