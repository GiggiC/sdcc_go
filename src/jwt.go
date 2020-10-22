package main

import (
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/twinj/uuid"
	"net/http"
	"os"
	"time"
)

type TokenDetails struct {
	AccessToken string
	AccessUuid  string
	AtExpires   int64
}

func CreateToken(email string) (*TokenDetails, error) {

	td := &TokenDetails{}
	td.AtExpires = time.Now().Local().Add(time.Minute * 15).Unix()
	td.AccessUuid = uuid.NewV4().String()

	err := os.Setenv("ACCESS_SECRET", "jdnfksdmfksd")

	if err != nil {
		panic(err)
	}

	atClaims := jwt.MapClaims{}
	atClaims["authorized"] = true
	atClaims["access_uuid"] = td.AccessUuid
	atClaims["email"] = email
	atClaims["exp"] = td.AtExpires

	at := jwt.NewWithClaims(jwt.SigningMethodHS256, atClaims)
	td.AccessToken, err = at.SignedString([]byte(os.Getenv("ACCESS_SECRET")))

	if err != nil {
		return nil, err
	}

	return td, nil
}

func CreateAuth(email string, td *TokenDetails) error {

	at := time.Unix(td.AtExpires, 0) //converting Unix to UTC(to Time object)
	now := time.Now().Local()

	errAccess := client.Set(ctx, td.AccessUuid, email, at.Sub(now)).Err()

	if errAccess != nil {

		return errAccess
	}

	return nil
}

func VerifyToken(c *gin.Context) (*jwt.Token, error) {

	tokenString := ExtractToken(c)

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {

		//Make sure that the token method conform to "SigningMethodHMAC"
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {

			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return []byte(os.Getenv("ACCESS_SECRET")), nil
	})

	if err != nil {

		return nil, err
	}

	return token, nil
}

func TokenValid(c *gin.Context) error {

	token, err := VerifyToken(c)

	if err != nil {

		return err
	}

	if _, ok := token.Claims.(jwt.Claims); !ok && !token.Valid {

		return err
	}

	return nil
}

func ExtractToken(c *gin.Context) string {

	accessToken, err := c.Request.Cookie("access_token")

	if err != nil || accessToken.Value == "" {

		return ""
	}

	return accessToken.Value
}

func ExtractTokenMetadata(c *gin.Context) (*AccessDetails, error) {

	token, err := VerifyToken(c)

	if err != nil {

		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)

	if ok && token.Valid {

		accessUuid, ok := claims["access_uuid"].(string)

		if !ok {

			return nil, err
		}

		email := fmt.Sprint(claims["email"])

		return &AccessDetails{
			AccessUuid: accessUuid,
			Email:      email,
		}, nil
	}

	return nil, err
}

func FetchAuth(authD *AccessDetails) error {

	_, err := client.Get(ctx, authD.AccessUuid).Result()

	if err != nil {
		return err
	}

	return nil
}

func DeleteAuth(givenUuid string) (int64, error) {

	deleted, err := client.Del(ctx, givenUuid).Result()

	if err != nil {
		return 0, err
	}

	return deleted, nil
}

func TokenAuthMiddleware() gin.HandlerFunc {

	return func(c *gin.Context) {
		err := TokenValid(c)

		if err != nil {

			redirect(c, "login.html", "not-logged", nil, false, http.StatusUnauthorized, "Login Page")
			c.Abort()
			return
		}

		c.Next()
	}
}
