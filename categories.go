package main

import (
	"regexp"
	"strings"
)

type Category string

const (
	CatFood          Category = "Food & Beverages"
	CatElectronics   Category = "Electronics & Accessories"
	CatAuto          Category = "Auto, Tools & Hardware"
	CatHome          Category = "Home, Kitchen & Cleaning"
	CatPersonalBaby  Category = "Personal Care, Baby & Health"
	CatSportsToys    Category = "Sports, Toys & Games"
	CatStationery    Category = "Stationery, Print & Books"
	CatApparel       Category = "Apparel & Fashion Accessories"
	CatPets          Category = "Pet Supplies"
	CatFestive       Category = "Festive, Gifts & Misc"
	CatUncategorized Category = "Uncategorized"
)

var categoryOrder = []Category{
	CatFood,
	CatElectronics,
	CatHome,
	CatPersonalBaby,
	CatAuto,
	CatSportsToys,
	CatApparel,
	CatStationery,
	CatFestive,
	CatPets,
	CatUncategorized,
}

var zeptoSubcategoryMap = map[string]Category{
	"142676b2-d759-4f6c-90e9-4fb21d5ad3fe": CatElectronics,
	"5a14eb17-b896-4a5c-b4ae-b3d7f4d61acd": CatAuto,
	"c2814f4b-969b-4255-9e49-578b4cad468a": CatAuto,
	"9d0a7e11-73b6-4cbb-b00b-108b2f65e59f": CatHome,
	"d74ddb01-0705-4cea-88f7-fe7ad668a85b": CatFestive,
	"b353a8f7-7dd6-4efe-954f-45bbddb6be5f": CatFood,
	"bc8ec1d3-95cf-4af5-aa60-43cde99798c1": CatFood,
	"414e9381-3ba3-4632-92e0-00fc29304900": CatFood,
	"22964a2b-0439-4236-9950-0d71b532b243": CatFood,
	"c6bba2b2-6b2d-4bd4-ae3d-adb6f9383393": CatFood,
	"dd3fd11a-5ed1-40a9-8e3c-edf3b82e7d32": CatFood,
	"bfd35e8f-39b8-483f-8e3a-b80231d614ce": CatFood,
	"df4f5100-c02f-4906-83b8-ddb744081a7a": CatFood,
	"b783925b-6b57-4750-99f6-01ea5145ec5e": CatFood,
	"cbacc78b-ae17-4324-8fd5-0ba540b10fb7": CatFood,
	"09e63c15-e5f7-4712-9ff8-513250b79942": CatFood,
	"dff3658b-c351-4e7f-8196-e98d0c66d99e": CatFood,
	"afce5895-f724-458f-8e23-28354a437c96": CatFood,
	"ca984d2d-70b8-464c-b182-41aa328b3d4b": CatFood,
	"d5fbe386-0579-4461-b88b-af427ffb31ea": CatFood,
	"1806412f-190a-46b1-be42-4237a4146eb1": CatFood,
	"5b162251-fc43-4ae4-b340-e588c5fadd68": CatHome,
	"97c725c2-41b3-4d78-ba3e-75e92b3ba054": CatHome,
	"f3b93fe0-9991-4786-bf5a-706485e9a7ab": CatPersonalBaby,
	"d2303c20-f52b-4c8c-9a7b-636a554b1748": CatSportsToys,
	"64e4511b-ea48-4131-a781-6ea85d8d695f": CatSportsToys,
	"ec911f07-e53f-4086-81e7-089a789860c8": CatFestive,
	"4a2fc9fc-3003-40b4-903e-a3baf6002221": CatFestive,
}

type categoryRule struct {
	cat Category
	re  *regexp.Regexp
}

var itemCategoryRules = []categoryRule{
	{CatFood, regexp.MustCompile(`(?i)\b(french fries)\b`)},
	{CatPets, regexp.MustCompile(`(?i)\b(pedigree|dog food|dog collar|dog leash|cat food|pet )\b`)},
	{CatStationery, regexp.MustCompile(`(?i)\b(a4 sheets|copier|excel bond|ball pen|brush pen|mechanical pencil|stationery|fevikwik|fevistik|glue stick|Passport Photos|Single side Portrait|My First Library|Sunderkand|Sherlock Holmes|Bhagwat Gita|Relativity|mounting tape|easy to tear tape)\b`)},
	{CatApparel, regexp.MustCompile(`(?i)\b(thermal top|thermal bottom|hand gloves|sleeveless vest|raincoat|sunglasses|lenskart|finger ring|knot studs|earrings|men bracelet|laptop backpack)\b`)},
	{CatSportsToys, regexp.MustCompile(`(?i)\b(badminton|shuttlecock|cricket bat|cricket ball|cricket kit|stumps with stand|basketball|monster truck|uno card|playing cards|toyzone|soft toys)\b`)},
	{CatAuto, regexp.MustCompile(`(?i)\b(car shampoo|liquid wax|car paint|steering knob|windshield shade|tyre inflator|chain lubricant|car freshener|car mobile holder|magnetic mobile holder|screw driver|soldering iron|plier|insulation tape|m-seal|steel round lock|single lock)\b`)},
	{CatElectronics, regexp.MustCompile(`(?i)\b(type-c|type c|usb-c|usb c|micro usb|aux cable|pen drive|sandisk|lightning|charging cable|multipurpose cable|hdmi cable|otg converter|usb hub|gan charger|charging adapter|wireless charger|car charger|power strip|extension board|earphones|headphones|wireless microphone|tripod stand|led light|led lights|led bulb|w bulb|batten|bulb holder|security camera|tapo c100|tapo c200|hard drive enclosure|wireless mouse|gaming mouse|smart watch|noisefit|gamepad|gps tracker|world tag|mobile cover|magsafe|screen gadget cleaner|alkaline battery|coin battery|zinc battery|table fan)\b`)},
	{CatPersonalBaby, regexp.MustCompile(`(?i)\b(diaper|mamypoko|pampers|bumtum|baby wipes|baby gift|baby lotion|body lotion|baby nibbler|baby rattle|baby plush|feeding bottle|baby soother|pacifier|breast pump|infant formula|follow up formula|follow-up formula|nan pro|dexolac|condom|durex|manforce|edible lubricant|pregnancy test|pregnancy detection|prega news|toothpaste|toothbrush|teeth whitener|dabur red|mouthwash|mouth spray|face wash|hand wash|bodywash|body wash|bath soap|glow soap|syndet soap|derma co|dot & key|moisturizer|lip balm|makeup|cosmetic|shampoo|hair mask|hair removal|sunscreen|skin care|rose water|cotton buds|nail clipper|band-aid|adhesive bandage|antacid|eno|inhaler|vicks|melatonin|shilajit|creatine|monohydrate|body warmer|heat pouch)\b`)},
	{CatHome, regexp.MustCompile(`(?i)\b(electric kettle|air fryer|mug warmer|wall fan|detergent|surf excel|tide|ariel|dishwash|vim|exo|toilet cleaner|toilet brush|harpic|scrub pad|scrubber|sponge scrub|duster cloth|kitchen cloth|microfiber cloth|garbage bag|face tissue|facial tissues|mosquito|all out|good knight|odomos|cockroach|hit .* spray|air freshener|aer power pocket|scented candle|spider plant|thermal bottle|sipper bottle|straw set|lunch box|kitchen knife|stainless steel knife|cookie jars|disposable plates|disposable glass|disposable spoon|party glass|baking & parchment paper|barbeque griller|natural coal|matchbox|ashtray|door seal)\b`)},
	{CatFestive, regexp.MustCompile(`(?i)\b(holi gulal|rakhi|pooja flowers|modak mould|google pay card|flyer|activity sheet)\b`)},
	{CatFood, regexp.MustCompile(`(?i)\b(milk|curd|paneer|tofu|cheese|butter|buttermilk|lassi|yogurt|kheer|rabri|panchamrit|condensed milk|milkmaid|bread|sourdough|pav|croissant|rusk|bun|cake|muffin|pancake|cookie|cookies|biscuit|biscuits|jim jam|little hearts|good day|dark fantasy|krackjack|parle-g|lotus biscoff|chips|wafers|crisps|nachos|rice cracker|kurkure|puffcorn|puffs|bingo|lay's|pringles|doritos|too yumm|uncle chipps|crax|cheetos|popcorn|namkeen|bhujia|mixture|party mix|murukku|gathiya|mathri|matthi|namak para|bhelpuri|peanuts|chana|soya sticks|makhana|foxnuts|sabudana|sago|almonds|cashew|cashewnut|pistachios|walnut|dates|apricots|trail mix|seeds mix|mix nuts|chikki|soan papdi|kaju katli|laddu|laddoo|burfi|gujia|mysore pak|peda|tamarind sweet|iml[i]|lollipop|chocolate|bournville|dairy milk|kitkat|munch|snickers|candy|eclair|chewing gum|orbit|ice cream|cornetto|soft drink|coca-cola|diet coke|diet-coke|pepsi|sprite|thums up|limca|mirinda|mountain dew|7up|nimbooz|appy|fizz|frooti|maaza|rooh afza|sharbat|juice|coconut water|tender coconut|sparkling|tonic water|soda|water|bisleri|kinley|energy drink|red bull|monster|gatorade|o'cean|budweiser|heineken|coolberg|non-alcoholic beer|cocktail mix|tea|coffee|movenpick|nescafe|bru|taj mahal|lipton|bevzilla|sleepy owl|rage coffee|colombian brew|percol|tang|glucon-d|centrum|probiotic|yakult|whey protein|whey max|protein powder|protein bar|protein wafer|liquid iv|noodles|maggi|nissin|indomie|koka|ramen|ramyun|samyang|otoki|pasta|macaroni|penne|yippee|vermicelli|seviyan|topokki|oats|poha|daliya|upma|soup|knorr|ready to eat|minute khana|veg biryani|baked beans|sweet corn|french fries|mccain|samosa|momos|kebab|patty|pizza pocket|eggs|eggoz|chicken|salami|sausage|nuggets|meatballs|pepperoni|ham|bacon|pork|licious|prasuma|godrej yummiez|itc master chef|chefigo|la carne|meatzza|meatigo|shaka harry|atta|maida|besan|sooji|rice|dal|rajma|kabuli|soya chunks|jowar|millets|sugar|khand|jaggery|salt|mustard oil|oil|ghee|vinegar|soy sauce|ketchup|mayonnaise|mustard sauce|chilli sauce|tabasco|spread|dressing|jam|honey|chocolate syrup|lemoneez|pickle|ginger garlic paste|turmeric|haldi|chilli powder|chilli flakes|garam masala|mutton masala|jeera|cumin|cardamom|elachi|cinnamon|dalchini|oregano|methi|saffron|mango|apple|banana|pineapple|watermelon|muskmelon|kiwi|pomegranate|grapes|orange|kinnow|guava|papaya|sapota|chikoo|litchi|dragon fruit|passion fruit|plum|cherry|jamun|strawberry|grapefruit|lemon|tomato|potato|sweet potato|carrot|kale|ginger|garlic|chilli|coriander|lettuce|mixed vegetables|ice cubes)\b`)},
}

func classifyItem(name, subcatID string) Category {
	if subcatID != "" {
		if cat, ok := zeptoSubcategoryMap[subcatID]; ok {
			return cat
		}
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return CatUncategorized
	}
	for _, r := range itemCategoryRules {
		if r.re.MatchString(trimmed) {
			return r.cat
		}
	}
	return CatUncategorized
}
