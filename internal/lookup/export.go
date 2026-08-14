package lookup

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

const (
	ExportMaxRows = 50000
	exportDash    = "—"
)

var exportHeadersHLR = []string{
	"Телефон", "Статус", "Результат", "Доступен", "Оператор", "Страна", "Регион",
	"MCC/MNC", "IMSI", "MSC", "Роуминг", "Страна роуминга", "Оператор роуминга",
	"Ошибки", "Подробности",
}

var exportHeadersPing = []string{
	"Телефон", "Статус", "Результат", "Доступен", "Подробности",
}

var exportItemStatusRU = map[string]string{
	"queued":    "в очереди",
	"reserved":  "зарезервирован",
	"sent":      "отправлен провайдеру",
	"pending":   "ждём ответ",
	"completed": "готов",
	"failed":    "ошибка",
	"cancelled": "отменён",
}

var exportResultRU = map[string]string{
	"reachable":   "в сети",
	"unreachable": "не в сети",
	"pending":     "в обработке",
	"error":       "ошибка проверки",
	"unknown":     "нет данных",
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

var exportItemError = map[string]string{
	"CHECK_TIMEOUT":               "Истекло время ожидания ответа провайдера",
	"QUEUE_DEAD_LETTER":           "Сбой очереди обработки",
	"MISSING_PROVIDER_MESSAGE_ID": "Нет идентификатора сообщения у провайдера",
	"RESERVED_STALE_TIMEOUT":      "Превышено время ожидания отправки",
	"CSV_EMPTY":                   "CSV не содержит номеров телефонов",
	"CSV_TOO_MANY_ROWS":           "CSV превышает лимит строк",
	"CSV_INVALID_PHONES":          "В CSV есть некорректные номера",
	"PRICE_SNAPSHOT_MISSING":      "Не задана цена тарифа",
	"CSV_PARSE_ABANDONED":         "Не удалось разобрать CSV",
	"check_timeout":               "Истекло время ожидания ответа провайдера",
	"reserved_stale_timeout":      "Превышено время ожидания отправки",
	"csv_parse_abandoned":         "Не удалось разобрать CSV",
	"csv_parse_failed":            "Не удалось разобрать CSV",
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
		exportResult(deref(item.ResultStatus)),
		exportBool(item.IsReachable),
	}
	if checkType == sqlcdb.LookupCheckTypeHlr {
		cells = append(cells,
			exportText(deref(item.OperatorName)),
			exportText(deref(item.CountryCode)),
			exportText(extraString(extras, "region")),
			exportMccMnc(item),
			exportText(deref(item.Imsi)),
			exportText(extraString(extras, "msc")),
			exportBool(item.Roaming),
			exportText(extraString(extras, "roaming_country", "roamingCountry")),
			exportText(extraString(extras, "roaming_operator", "roamingOperator")),
			exportProviderError(item),
		)
	}
	cells = append(cells, exportDetails(item))
	return cells
}

func BuildXLSX(checkType sqlcdb.LookupCheckType, items []sqlcdb.LookupItem) ([]byte, error) {
	headers := ExportHeaders(checkType)
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	writeXLSXRow(&sheet, 1, headers)
	for i, item := range items {
		writeXLSXRow(&sheet, i+2, ExportRow(checkType, item))
	}
	sheet.WriteString(`</sheetData></worksheet>`)

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`,
		"xl/workbook.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<sheets><sheet name="items" sheetId="1" r:id="rId1"/></sheets>
</workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
</Relationships>`,
		"xl/worksheets/sheet1.xml": sheet.String(),
	}
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(body)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeXLSXRow(b *strings.Builder, row int, cells []string) {
	fmt.Fprintf(b, `<row r="%d">`, row)
	for i, cell := range cells {
		col := xlsxCol(i)
		fmt.Fprintf(b, `<c r="%s%d" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, col, row, xlsxEscape(cell))
	}
	b.WriteString(`</row>`)
}

func xlsxCol(i int) string {
	s := ""
	i++
	for i > 0 {
		i--
		s = string(rune('A'+i%26)) + s
		i /= 26
	}
	return s
}

func xlsxEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
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

func exportResult(result string) string {
	if result == "" {
		return exportDash
	}
	if v, ok := exportResultRU[result]; ok {
		return v
	}
	return result
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

func exportMccMnc(item sqlcdb.LookupItem) string {
	if item.Mcc == nil && item.Mnc == nil {
		return exportDash
	}
	return exportText(deref(item.Mcc)) + "/" + exportText(deref(item.Mnc))
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

func exportDetails(item sqlcdb.LookupItem) string {
	code := strings.TrimSpace(deref(item.ErrorCode))
	if code != "" {
		if v, ok := exportItemError[code]; ok {
			return v
		}
		if v, ok := exportItemError[strings.ToUpper(code)]; ok {
			return v
		}
	}
	msg := sanitizeProviderBrand(deref(item.ErrorMessage))
	if msg == "" {
		return exportDash
	}
	return msg
}

func sanitizeProviderBrand(s string) string {
	if s == "" {
		return ""
	}
	replacer := strings.NewReplacer("SMSC.ru", "провайдер", "smsc.ru", "провайдер", "SMSC", "провайдер", "smsc", "провайдер")
	return replacer.Replace(s)
}
