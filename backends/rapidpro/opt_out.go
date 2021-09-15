package rapidpro

import "strings"

var keywords = [...]string{
	"exit", "quit", "unsubscribe", "cancel", "stop", "stopall", "end", "remove", "salir", "parar", "cancelar",
	"detener", "bloquear", "terminar", "joder", "mierda", "eliminar", "退出", "退订", "取消", "停止", "全部停止",
	"结束", "删除", "thoát", "hủy", "bỏ", "đăng", "dừng", "kết", "thúc", "xóa", "mag-exit", "mag-unsubscribe",
	"kanselahin", "itigil", "itigillahat", "wakasan", "alisin", "lumabas", "종료", "구독취소", "취소", "중지", "모두중지", "끝",
	"꺼져", "개", "제거", "망할", "退出", "나가기", "fuck", "fucking", "fucker", "fuckers", "motherfucker", "motherfuckers",
	"phuck", "duck", "ducking", "shit", "bullshit", "bitch", "bitches", "asshole", "assholes", "faggot", "cunt",
	"cunts", "pussy", "pussies", "dick", "dicks", "retard", "retards", "damn", "piss", "suck", "pendeja", "pendejo",
	"puto", "puta", "illegal", "spam", "spammer", "spamming", "unsolicited", "annoying", "jesus", "shut",
	"unsubscribed", "harass", "harassed", "fuck", "fucking", "fucker", "fuckers", "motherfucker", "motherfuckers",
	"phuck", "duck", "ducking", "shit", "bullshit", "bitch", "bitches", "asshole", "assholes", "faggot", "cunt",
	"cunts", "pussy", "pussies", "dick", "dicks", "retard", "retards", "damn", "piss", "suck", "pendeja", "pendejo",
	"puto", "puta", "illegal", "spam", "spammer", "spamming", "unsolicited", "annoying", "jesus", "shut",
}

func checkOptOutKeywordPresence(text string) bool {
	loweredText := strings.ToLower(text)
	for _, keyword := range keywords {
		if strings.Contains(loweredText, keyword) {
			return true
		}
	}
	return false
}
