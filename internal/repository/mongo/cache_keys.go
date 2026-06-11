package mongo

import "fmt"

func userIDKey(id string) string {
	return fmt.Sprintf("user:id:%s", id)
}

func userEmailKey(email string) string {
	return fmt.Sprintf("user:email:%s", email)
}

func sessionIDKey(id string) string {
	return fmt.Sprintf("session:id:%s", id)
}

func sessionRefreshKey(hash string) string {
	return fmt.Sprintf("session:refresh:%s", hash)
}

func userActiveSessionsKey(userID string) string {
	return fmt.Sprintf("session:user:%s:active", userID)
}
