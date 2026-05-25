package utilities

import (
	"context"
	"strings"
)

type VASCatalog struct {
	Source            string        `json:"source"`
	AirtimeNetworks   []CatalogItem `json:"airtime_networks"`
	DataPlans         []DataPlan    `json:"data_plans"`
	ElectricityDiscos []CatalogItem `json:"electricity_discos"`
	BettingProviders  []CatalogItem `json:"betting_providers"`
}

type CatalogItem struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	Category   string `json:"category,omitempty"`
	BillerCode string `json:"biller_code,omitempty"`
	ItemCode   string `json:"item_code,omitempty"`
	LabelName  string `json:"label_name,omitempty"`
}

type DataPlan struct {
	Code        string `json:"code"`
	Network     string `json:"network"`
	Name        string `json:"name"`
	Amount      int64  `json:"amount"`
	Validity    string `json:"validity"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	BillerCode  string `json:"biller_code,omitempty"`
	ItemCode    string `json:"item_code,omitempty"`
}

type CatalogProvider interface {
	FetchCatalog(ctx context.Context) (*FlutterwaveCatalog, error)
}

type FlutterwaveCatalog struct {
	Categories []FlutterwaveBillCategory
	Billers    map[string][]FlutterwaveBiller
	Items      map[string][]FlutterwaveBillItem
}

func (s *Service) GetCatalog(ctx context.Context) VASCatalog {
	if provider, ok := s.bills.(CatalogProvider); ok {
		catalog, err := provider.FetchCatalog(ctx)
		if err == nil {
			if mapped := mapFlutterwaveCatalog(catalog); !mapped.isEmpty() {
				mapped.Source = "flutterwave"
				return mapped
			}
		}
		if err != nil && s.log != nil {
			s.log.Warn("Flutterwave catalog lookup failed; using fallback catalog")
		}
	}
	return fallbackCatalog()
}

func fallbackCatalog() VASCatalog {
	return VASCatalog{
		Source: "fallback",
		AirtimeNetworks: []CatalogItem{
			{Code: "MTN", Name: "MTN Nigeria"},
			{Code: "GLO", Name: "Globacom"},
			{Code: "AIRTEL", Name: "Airtel Nigeria"},
			{Code: "9MOBILE", Name: "9mobile"},
		},
		DataPlans: []DataPlan{
			{Code: "MTN_1GB_30D", Network: "MTN", Name: "1GB", Amount: 50000, Validity: "30 days"},
			{Code: "MTN_2GB_30D", Network: "MTN", Name: "2GB", Amount: 100000, Validity: "30 days"},
			{Code: "GLO_1GB_30D", Network: "GLO", Name: "1GB", Amount: 50000, Validity: "30 days"},
			{Code: "AIRTEL_1GB_30D", Network: "AIRTEL", Name: "1GB", Amount: 50000, Validity: "30 days"},
			{Code: "9MOBILE_1GB_30D", Network: "9MOBILE", Name: "1GB", Amount: 50000, Validity: "30 days"},
		},
		ElectricityDiscos: []CatalogItem{
			{Code: "IKEDC", Name: "Ikeja Electric"},
			{Code: "EKEDC", Name: "Eko Electricity Distribution"},
			{Code: "AEDC", Name: "Abuja Electricity Distribution"},
			{Code: "PHED", Name: "Port Harcourt Electricity Distribution"},
			{Code: "KEDCO", Name: "Kano Electricity Distribution"},
			{Code: "IBEDC", Name: "Ibadan Electricity Distribution"},
			{Code: "BEDC", Name: "Benin Electricity Distribution"},
			{Code: "EEDC", Name: "Enugu Electricity Distribution"},
			{Code: "JED", Name: "Jos Electricity Distribution"},
			{Code: "KAEDCO", Name: "Kaduna Electric"},
			{Code: "YEDC", Name: "Yola Electricity Distribution"},
		},
		BettingProviders: []CatalogItem{
			{Code: "BET9JA", Name: "Bet9ja"},
			{Code: "SPORTYBET", Name: "SportyBet"},
			{Code: "BETKING", Name: "BetKing"},
			{Code: "NAIRABET", Name: "NairaBET"},
			{Code: "1XBET", Name: "1xBet"},
		},
	}
}

func mapFlutterwaveCatalog(catalog *FlutterwaveCatalog) VASCatalog {
	out := VASCatalog{}
	if catalog == nil {
		return out
	}

	for _, category := range catalog.Categories {
		categoryCode := category.Code()
		billers := catalog.Billers[categoryCode]
		switch {
		case isAirtimeCategory(category):
			for _, biller := range billers {
				out.AirtimeNetworks = append(out.AirtimeNetworks, CatalogItem{
					Code:       displayCode(biller.Code()),
					Name:       biller.Name,
					Category:   categoryCode,
					BillerCode: biller.Code(),
					LabelName:  biller.LabelName,
				})
			}
		case isDataCategory(category):
			for _, biller := range billers {
				for _, item := range catalog.Items[biller.Code()] {
					out.DataPlans = append(out.DataPlans, DataPlan{
						Code:        item.Code(),
						Network:     biller.Name,
						Name:        item.Name,
						Amount:      item.AmountKobo(),
						Validity:    item.Validity,
						Description: item.Description,
						Category:    categoryCode,
						BillerCode:  biller.Code(),
						ItemCode:    item.Code(),
					})
				}
			}
		case isElectricityCategory(category):
			for _, biller := range billers {
				out.ElectricityDiscos = append(out.ElectricityDiscos, CatalogItem{
					Code:       displayCode(biller.Code()),
					Name:       biller.Name,
					Category:   categoryCode,
					BillerCode: biller.Code(),
					LabelName:  biller.LabelName,
				})
			}
		case isBettingCategory(category):
			for _, biller := range billers {
				out.BettingProviders = append(out.BettingProviders, CatalogItem{
					Code:       displayCode(biller.Code()),
					Name:       biller.Name,
					Category:   categoryCode,
					BillerCode: biller.Code(),
					LabelName:  biller.LabelName,
				})
			}
		}
	}
	return out
}

func (c VASCatalog) isEmpty() bool {
	return len(c.AirtimeNetworks) == 0 &&
		len(c.DataPlans) == 0 &&
		len(c.ElectricityDiscos) == 0 &&
		len(c.BettingProviders) == 0
}

func isAirtimeCategory(category FlutterwaveBillCategory) bool {
	return containsAny(category.SearchText(), "airtime")
}

func isDataCategory(category FlutterwaveBillCategory) bool {
	return containsAny(category.SearchText(), "data", "mobiledata", "mobile data")
}

func isElectricityCategory(category FlutterwaveBillCategory) bool {
	return containsAny(category.SearchText(), "utility", "electric", "power")
}

func isBettingCategory(category FlutterwaveBillCategory) bool {
	return containsAny(category.SearchText(), "bet", "gaming")
}

func containsAny(value string, needles ...string) bool {
	value = strings.ToLower(value)
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func displayCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return "UNKNOWN"
	}
	return strings.ToUpper(code)
}
