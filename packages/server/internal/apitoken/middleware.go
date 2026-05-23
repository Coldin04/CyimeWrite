package apitoken

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

const (
	LocalsTokenID = "apiTokenId"
	LocalsScopes  = "apiTokenScopes"
)

func Protected(requiredScopes ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		rawToken, err := bearerToken(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
		}

		authenticated, err := Authenticate(rawToken, c.IP())
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
		}
		if !HasScopes(authenticated.Scopes, requiredScopes...) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error":          "insufficient API token scope",
				"requiredScopes": requiredScopes,
			})
		}

		c.Locals("userId", authenticated.UserID.String())
		c.Locals(LocalsTokenID, authenticated.TokenID.String())
		c.Locals(LocalsScopes, authenticated.Scopes)
		return c.Next()
	}
}

func bearerToken(c *fiber.Ctx) (string, error) {
	authHeader := strings.TrimSpace(c.Get("Authorization"))
	if authHeader == "" {
		return "", fiber.NewError(fiber.StatusUnauthorized, "missing Authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" || strings.TrimSpace(parts[1]) == "" {
		return "", fiber.NewError(fiber.StatusUnauthorized, "malformed Authorization header")
	}
	return strings.TrimSpace(parts[1]), nil
}
