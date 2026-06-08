package fluvpay

// String devolve um ponteiro para a string informada. Útil para preencher
// campos opcionais dos parâmetros (Description, AffiliateCode, etc).
func String(v string) *string { return &v }

// Int devolve um ponteiro para o inteiro informado. Útil para campos opcionais
// como ExpiresInSeconds.
func Int(v int) *int { return &v }

// Bool devolve um ponteiro para o booleano informado. Útil para apontar
// explicitamente um campo opcional como PassFeeToPayer.
func Bool(v bool) *bool { return &v }
