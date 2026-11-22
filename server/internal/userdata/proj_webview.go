package userdata

import (
	"lunar-tear/server/internal/bunkercache"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/utils"
)

var webviewPanelMissionCatalog = bunkercache.Value(masterdata.LoadWebviewPanelMissionCatalog)

func init() {
	register("IUserWebviewPanelMission", func(user store.UserState) string {
		if PanelPagesDone != nil {
			return panelPagesDone(user)
		}
		pageIds := webviewPanelMissionCatalog().PageIds
		records := make([]map[string]any, 0, len(pageIds))
		for _, pageId := range pageIds {
			records = append(records, map[string]any{
				"userId":                    user.UserId,
				"webviewPanelMissionPageId": pageId,
				"rewardReceiveDatetime":     user.GameStartDatetime,
				"latestVersion":             user.GameStartDatetime,
			})
		}
		s, _ := utils.EncodeJSONMaps(records...)
		return s
	})
}
