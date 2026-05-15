package dto

type TranslateRequest struct {
	Text       string `json:"text"`
	TargetLang string `json:"target_lang"`
}

type TranslateResponse struct {
	Result string `json:"result"`
}
