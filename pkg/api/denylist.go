package api

// PIIDenylist defines a best-effort list of lowercase keys containing sensitive data.
var PIIDenylist = map[string]bool{
	// credentials and auth
	"password":      true,
	"passwd":        true,
	"pwd":           true,
	"passphrase":    true,
	"secret":        true,
	"token":         true,
	"api_key":       true,
	"apikey":        true,
	"access_token":  true,
	"refresh_token": true,
	"private_key":   true,
	"client_secret": true,

	// financial data
	"credit_card": true,
	"card_number": true,
	"card_num":    true,
	"cc":          true,
	"cc_num":      true,
	"cvv":         true,
	"cvc":         true,
	"routing_num": true,
	"account_num": true,
	"iban":        true,

	// government and national identifiers
	"ssn":             true,
	"social_security": true,
	"tax_id":          true,
	"tin":             true,
	"passport":        true,
	"driver_license":  true,
	"dl_number":       true,

	// personal and contact information
	"address":       true,
	"email":         true,
	"email_addr":    true,
	"phone":         true,
	"phone_num":     true,
	"telephone":     true,
	"mobile":        true,
	"cell":          true,
	"dob":           true,
	"date_of_birth": true,
	"birthdate":     true,
	"first_name":    true,
	"last_name":     true,
	"surname":       true,
	"full_name":     true,
}
