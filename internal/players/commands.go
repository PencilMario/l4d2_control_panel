package players

import (
	"errors"
	"fmt"
)

func Kick(userID int) (string, error) {
	if userID < 1 {
		return "", errors.New("invalid user id")
	}
	return fmt.Sprintf("kickid %d", userID), nil
}
func Ban(userID, minutes int) (string, error) {
	if userID < 1 || minutes < 0 {
		return "", errors.New("invalid ban parameters")
	}
	return fmt.Sprintf("banid %d %d kick; writeid", minutes, userID), nil
}
