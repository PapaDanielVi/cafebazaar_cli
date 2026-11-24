package publisher

import (
	"context"
	"fmt"
)

const (
	// CategoryGroupApp identifies standard Android application categories on CafeBazaar.
	CategoryGroupApp = "app"
	// CategoryGroupGame identifies Android game categories on CafeBazaar.
	CategoryGroupGame = "game"
)

// Category represents an official CafeBazaar app or game category and its API category_id.
type Category struct {
	ID     int    `json:"id" yaml:"id"`
	NameFA string `json:"name_fa" yaml:"name_fa"`
	NameEN string `json:"name_en" yaml:"name_en"`
	Group  string `json:"group" yaml:"group"`
}

// FormatLabel formats a category for display in interactive selectors and confirmation summaries.
func (c Category) FormatLabel(ctx context.Context) string {
	_ = ctx
	groupLabel := "App"
	if c.Group == CategoryGroupGame {
		groupLabel = "Game"
	}
	return fmt.Sprintf("[%2d] %-16s  %s (%s)", c.ID, c.NameEN, c.NameFA, groupLabel)
}

var officialCategories = []Category{
	// App categories (برنامه‌ها).
	{ID: 11, NameFA: "ابزارها", NameEN: "Tools", Group: CategoryGroupApp},
	{ID: 20, NameFA: "آب و هوا", NameEN: "Weather", Group: CategoryGroupApp},
	{ID: 62, NameFA: "آشپزی و رستوران", NameEN: "Food-drink", Group: CategoryGroupApp},
	{ID: 16, NameFA: "آموزش", NameEN: "Education", Group: CategoryGroupApp},
	{ID: 24, NameFA: "امور مالی", NameEN: "Finance", Group: CategoryGroupApp},
	{ID: 17, NameFA: "پزشکی", NameEN: "Medical", Group: CategoryGroupApp},
	{ID: 26, NameFA: "تناسب اندام", NameEN: "Health-fitness", Group: CategoryGroupApp},
	{ID: 13, NameFA: "خرید", NameEN: "Shopping", Group: CategoryGroupApp},
	{ID: 41, NameFA: "رفت و آمد", NameEN: "Maps-navigation", Group: CategoryGroupApp},
	{ID: 23, NameFA: "سبک زندگی", NameEN: "Lifestyle", Group: CategoryGroupApp},
	{ID: 2, NameFA: "سرگرمی", NameEN: "Entertainment", Group: CategoryGroupApp},
	{ID: 10, NameFA: "سفر", NameEN: "Travel-local", Group: CategoryGroupApp},
	{ID: 14, NameFA: "شبکه\u200cهای اجتماعی", NameEN: "Social", Group: CategoryGroupApp},
	{ID: 18, NameFA: "شخصی\u200cسازی", NameEN: "Personalization", Group: CategoryGroupApp},
	{ID: 22, NameFA: "عکاسی و ویدیو", NameEN: "Photography", Group: CategoryGroupApp},
	{ID: 9, NameFA: "کتاب\u200cها و مطبوعات", NameEN: "Books-reference", Group: CategoryGroupApp},
	{ID: 64, NameFA: "کودک", NameEN: "Kids-apps", Group: CategoryGroupApp},
	{ID: 42, NameFA: "مذهبی", NameEN: "Religious", Group: CategoryGroupApp},
	{ID: 19, NameFA: "موسیقی", NameEN: "Music-audio", Group: CategoryGroupApp},
	{ID: 15, NameFA: "ورزشی", NameEN: "Sports", Group: CategoryGroupApp},

	// Game categories (بازی‌ها).
	{ID: 35, NameFA: "اکشن", NameEN: "Action", Group: CategoryGroupGame},
	{ID: 58, NameFA: "استراتژی", NameEN: "Strategy", Group: CategoryGroupGame},
	{ID: 46, NameFA: "امتیازی", NameEN: "Arcade", Group: CategoryGroupGame},
	{ID: 33, NameFA: "تفننی", NameEN: "Casual", Group: CategoryGroupGame},
	{ID: 37, NameFA: "رانندگی", NameEN: "Racing", Group: CategoryGroupGame},
	{ID: 56, NameFA: "شبیه\u200cسازی", NameEN: "Simulation", Group: CategoryGroupGame},
	{ID: 59, NameFA: "کلمات و دانستنی\u200cها", NameEN: "Word-trivia", Group: CategoryGroupGame},
	{ID: 63, NameFA: "کودک", NameEN: "Kids-games", Group: CategoryGroupGame},
	{ID: 36, NameFA: "معمایی", NameEN: "Puzzle", Group: CategoryGroupGame},
	{ID: 34, NameFA: "ورزشی", NameEN: "Sports-game", Group: CategoryGroupGame},
}

// Categories returns a copy of all official CafeBazaar app and game categories.
func Categories(ctx context.Context) []Category {
	_ = ctx
	out := make([]Category, len(officialCategories))
	copy(out, officialCategories)
	return out
}

// FindCategoryByID looks up an official CafeBazaar category by its numeric category_id.
func FindCategoryByID(ctx context.Context, id int) (Category, bool) {
	_ = ctx
	for _, c := range officialCategories {
		if c.ID == id {
			return c, true
		}
	}
	return Category{}, false
}
