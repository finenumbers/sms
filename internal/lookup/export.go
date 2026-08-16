package lookup

import (
	"strings"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/xlsxexport"
)

const (
	ExportMaxRows = 50000
	exportDash    = "—"
)

var exportHeadersHLR = []string{
	"Номер", "Статус", "Результат", "Оператор", "Страна", "Регион",
	"MCC", "MNC", "IMSI", "MSC", "Роуминг", "Страна роуминга", "Оператор роуминга",
	"Ошибка",
}

var exportHeadersPing = []string{
	"Номер", "Статус", "Результат", "Ошибка",
}

var exportItemStatusRU = map[string]string{
	"queued":    "в очереди",
	"reserved":  "резерв",
	"sent":      "отправлен",
	"pending":   "ожидание",
	"completed": "готово",
	"failed":    "ошибка",
	"cancelled": "отменён",
}

var exportResultRU = map[string]string{
	"reachable":   "в сети",
	"unreachable": "не в сети",
	"pending":     "ожидание",
	"error":       "ошибка",
	"unknown":     "неизвестно",
}

var exportProviderStatusErr = map[string]string{
	"0": "Нет ошибки", "1": "Абонент не существует", "6": "Абонент не в сети",
	"11": "Не подключена услуга", "12": "Ошибка в телефоне абонента", "13": "Абонент заблокирован",
	"21": "Нет поддержки сервиса", "200": "Виртуальная отправка", "219": "Замена sim-карты",
	"220": "Переполнена очередь у оператора", "237": "Абонент не отвечает", "238": "Нет шаблона",
	"239": "Запрещенный ip-адрес", "240": "Абонент занят", "241": "Ошибка конвертации",
	"242": "Зафиксирован автоответчик", "243": "Не заключен договор", "244": "Рассылка запрещена",
	"245": "Статус не получен", "246": "Ограничение по времени", "247": "Превышен лимит сообщений",
	"248": "Нет маршрута", "249": "Неверный формат номера", "250": "Номер запрещен настройками",
	"251": "Превышен лимит на один номер", "252": "Номер запрещен", "253": "Запрещено спам-фильтром",
	"254": "Незарегистрированный sender id", "255": "Отклонено оператором",
}

var exportProviderAPIErr = map[string]string{
	"1": "Неверные параметры запроса", "2": "Ошибка авторизации у провайдера",
	"3": "Недостаточно средств у провайдера", "4": "IP-адрес заблокирован провайдером",
	"5": "Некорректная дата в запросе", "6": "Запрещено провайдером",
	"7": "Неверный номер телефона", "8": "Невозможно доставить / проверить номер",
	"9": "Слишком много одинаковых запросов",
}

func ExportHeaders(checkType sqlcdb.LookupCheckType) []string {
	if checkType == sqlcdb.LookupCheckTypeHlr {
		return append([]string(nil), exportHeadersHLR...)
	}
	return append([]string(nil), exportHeadersPing...)
}

func ExportRow(checkType sqlcdb.LookupCheckType, item sqlcdb.LookupItem) []string {
	extras := extrasFromItem(item)
	cells := []string{
		exportText(item.PhoneE164),
		exportStatus(string(item.Status)),
		exportResultCell(item),
	}
	if checkType == sqlcdb.LookupCheckTypeHlr {
		cells = append(cells,
			exportText(deref(item.OperatorName)),
			exportText(deref(item.CountryCode)),
			exportText(extraString(extras, "region")),
			exportText(deref(item.Mcc)),
			exportText(deref(item.Mnc)),
			exportText(deref(item.Imsi)),
			exportText(extraString(extras, "msc")),
			exportBool(item.Roaming),
			exportText(extraString(extras, "roaming_country", "roamingCountry")),
			exportText(extraString(extras, "roaming_operator", "roamingOperator")),
		)
	}
	cells = append(cells, exportError(item))
	return cells
}

func BuildXLSX(checkType sqlcdb.LookupCheckType, items []sqlcdb.LookupItem) ([]byte, error) {
	headers := ExportHeaders(checkType)
	rows := make([][]string, len(items))
	styles := make([]xlsxexport.RowStyle, len(items))
	for i, item := range items {
		rows[i] = ExportRow(checkType, item)
		if checkType == sqlcdb.LookupCheckTypeHlr && exportHLRNegativeRow(item) {
			styles[i] = xlsxexport.RowStyle{FillRGB: "FEF2F2", FontRGB: "991B1B"}
		}
	}
	return xlsxexport.BuildStyled("items", headers, rows, styles)
}

// exportHLRNegativeRow matches the cabinet/admin table: bg-red-50 text-red-800.
func exportHLRNegativeRow(item sqlcdb.LookupItem) bool {
	switch deref(item.ResultStatus) {
	case "unreachable", "error":
		return true
	}
	return item.Status == sqlcdb.LookupItemStatusFailed
}

func exportText(s string) string {
	if strings.TrimSpace(s) == "" {
		return exportDash
	}
	return s
}

func exportStatus(status string) string {
	if status == "" {
		return exportDash
	}
	if v, ok := exportItemStatusRU[status]; ok {
		return v
	}
	return status
}

func exportResultCell(item sqlcdb.LookupItem) string {
	if result := deref(item.ResultStatus); result != "" {
		if v, ok := exportResultRU[result]; ok {
			return v
		}
		return result
	}
	return exportBool(item.IsReachable)
}

func exportBool(v *bool) string {
	if v == nil {
		return exportDash
	}
	if *v {
		return "да"
	}
	return "нет"
}

func exportError(item sqlcdb.LookupItem) string {
	if msg := strings.TrimSpace(deref(item.ErrorMessage)); msg != "" {
		return msg
	}
	return exportProviderError(item)
}

func exportProviderError(item sqlcdb.LookupItem) string {
	code := strings.TrimSpace(deref(item.ErrorCode))
	if code == "" {
		return exportDash
	}
	if item.Status == sqlcdb.LookupItemStatusFailed && deref(item.ResultStatus) == "" {
		if v, ok := exportProviderAPIErr[code]; ok {
			return v
		}
	}
	if v, ok := exportProviderStatusErr[code]; ok {
		return v
	}
	return code
}
