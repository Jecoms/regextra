package regextra

import (
	"fmt"
	"reflect"
	"regexp"
	"testing"
)

func TestFindNamed(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		target    string
		groupName string
		want      string
		wantFound bool
	}{
		{
			name:      "found first group",
			pattern:   `(?P<first>one) (?P<second>two) (?P<second>again) three`,
			target:    "one two again three",
			groupName: "first",
			want:      "one",
			wantFound: true,
		},
		{
			name:      "found second group",
			pattern:   `(?P<first>one) (?P<second>two) (?P<second>again) three`,
			target:    "one two again three",
			groupName: "second",
			want:      "two",
			wantFound: true,
		},
		{
			name:      "group not found",
			pattern:   `(?P<first>one) (?P<second>two) (?P<second>again) three`,
			target:    "one two again three",
			groupName: "third",
			want:      "",
			wantFound: false,
		},
		{
			name:      "extract price",
			pattern:   `(?P<price>\$\d+(,\d{3})*(\.\d{1,2})?)`,
			target:    "The price is $1,234.56",
			groupName: "price",
			want:      "$1,234.56",
			wantFound: true,
		},
		{
			name:      "no match returns empty",
			pattern:   `(?P<name>[a-z]+)`,
			target:    "123",
			groupName: "name",
			want:      "",
			wantFound: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			got, found := FindNamed(re, tt.target, tt.groupName)
			if got != tt.want {
				t.Errorf("FindNamed() got = %v, want %v", got, tt.want)
			}
			if found != tt.wantFound {
				t.Errorf("FindNamed() found = %v, wantFound %v", found, tt.wantFound)
			}
		})
	}
}

func TestNamedGroups(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		target  string
		want    map[string]string
	}{
		{
			name:    "found multiple named groups",
			pattern: `(?P<first>one) (?P<second>two) (?P<second>again) three`,
			target:  "one two again three",
			want: map[string]string{
				"first":  "one",
				"second": "again", // duplicate names return last match
			},
		},
		{
			name:    "no match returns empty map",
			pattern: `(?P<first>one) (?P<second>two) (?P<second>again) three`,
			target:  "one two three",
			want:    map[string]string{},
		},
		{
			name:    "single named group",
			pattern: `(?P<word>\w+)`,
			target:  "hello",
			want: map[string]string{
				"word": "hello",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			if got := NamedGroups(re, tt.target); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NamedGroups() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ExampleFindNamed() {
	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
	name, ok := FindNamed(re, "Alice 30", "name")
	fmt.Printf("%s: %v\n", name, ok)
	// Output: Alice: true
}

func ExampleNamedGroups() {
	re := regexp.MustCompile(`(?P<year>\d{4})-(?P<month>\d{2})-(?P<day>\d{2})`)
	groups := NamedGroups(re, "Date: 2025-10-04")

	// Note: map iteration order is not guaranteed, so we print sorted
	keys := []string{"year", "month", "day"}
	for _, key := range keys {
		fmt.Printf("%s=%s ", key, groups[key])
	}
	// Output: year=2025 month=10 day=04
}

func TestAllNamedGroups(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		target  string
		want    map[string][]string
	}{
		{
			name:    "duplicate group names in same match",
			pattern: `(?P<word>\w+) (?P<word>\w+)`,
			target:  "hello world",
			want: map[string][]string{
				"word": {"hello", "world"},
			},
		},
		{
			name:    "multiple different groups",
			pattern: `(?P<name>\w+) (?P<age>\d+)`,
			target:  "Alice 30",
			want: map[string][]string{
				"name": {"Alice"},
				"age":  {"30"},
			},
		},
		{
			name:    "three duplicate group names",
			pattern: `(?P<item>\w+) (?P<item>\w+) (?P<item>\w+)`,
			target:  "one two three",
			want: map[string][]string{
				"item": {"one", "two", "three"},
			},
		},
		{
			name:    "mixed duplicate and unique groups",
			pattern: `(?P<word>\w+) (?P<num>\d+) (?P<word>\w+)`,
			target:  "hello 123 world",
			want: map[string][]string{
				"word": {"hello", "world"},
				"num":  {"123"},
			},
		},
		{
			name:    "no match returns empty map",
			pattern: `(?P<digit>\d+)`,
			target:  "abc",
			want:    map[string][]string{},
		},
		{
			name:    "single group single match",
			pattern: `(?P<price>\$\d+\.\d{2})`,
			target:  "Total: $19.99",
			want: map[string][]string{
				"price": {"$19.99"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			got := AllNamedGroups(re, tt.target)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AllNamedGroups() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ExampleAllNamedGroups() {
	re := regexp.MustCompile(`(?P<word>\w+) (?P<word>\w+) (?P<word>\w+)`)
	allGroups := AllNamedGroups(re, "one two three")

	fmt.Printf("word: %v\n", allGroups["word"])
	// Output: word: [one two three]
}

func TestUnmarshal(t *testing.T) {
	t.Run("basic string fields", func(t *testing.T) {
		type Person struct {
			Name string
			Age  string
		}
		re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
		var person Person
		err := Unmarshal(re, "Alice is 30", &person)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if person.Name != "Alice" {
			t.Errorf("Name = %q, want %q", person.Name, "Alice")
		}
		if person.Age != "30" {
			t.Errorf("Age = %q, want %q", person.Age, "30")
		}
	})

	t.Run("int type conversion", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
		var person Person
		err := Unmarshal(re, "Bob is 25", &person)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if person.Name != "Bob" {
			t.Errorf("Name = %q, want %q", person.Name, "Bob")
		}
		if person.Age != 25 {
			t.Errorf("Age = %d, want %d", person.Age, 25)
		}
	})

	t.Run("float type conversion", func(t *testing.T) {
		type Product struct {
			Name  string
			Price float64
		}
		re := regexp.MustCompile(`(?P<name>\w+) costs \$(?P<price>[\d.]+)`)
		var product Product
		err := Unmarshal(re, "Widget costs $19.99", &product)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if product.Name != "Widget" {
			t.Errorf("Name = %q, want %q", product.Name, "Widget")
		}
		if product.Price != 19.99 {
			t.Errorf("Price = %f, want %f", product.Price, 19.99)
		}
	})

	t.Run("struct tags for custom mapping", func(t *testing.T) {
		type Email struct {
			Username string `regex:"user"`
			Domain   string `regex:"domain"`
		}
		re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
		var email Email
		err := Unmarshal(re, "alice@example.com", &email)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if email.Username != "alice" {
			t.Errorf("Username = %q, want %q", email.Username, "alice")
		}
		if email.Domain != "example.com" {
			t.Errorf("Domain = %q, want %q", email.Domain, "example.com")
		}
	})

	t.Run("case insensitive field matching", func(t *testing.T) {
		type Data struct {
			UserName string
			Age      int
		}
		re := regexp.MustCompile(`(?P<username>\w+) (?P<age>\d+)`)
		var data Data
		err := Unmarshal(re, "john 42", &data)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if data.UserName != "john" {
			t.Errorf("UserName = %q, want %q", data.UserName, "john")
		}
		if data.Age != 42 {
			t.Errorf("Age = %d, want %d", data.Age, 42)
		}
	})

	t.Run("no match returns no error", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>[a-z]+)`) // Only lowercase letters
		var person Person
		err := Unmarshal(re, "123", &person)
		if err != nil {
			t.Errorf("Unmarshal() error = %v, want nil", err)
		}
		if person.Name != "" {
			t.Errorf("Name = %q, want empty string", person.Name)
		}
	})

	t.Run("error on non-pointer", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var person Person
		err := Unmarshal(re, "Alice", person) // Not a pointer
		if err == nil {
			t.Error("Unmarshal() expected error for non-pointer, got nil")
		}
	})

	t.Run("error on nil pointer", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var person *Person
		err := Unmarshal(re, "Alice", person) // Nil pointer
		if err == nil {
			t.Error("Unmarshal() expected error for nil pointer, got nil")
		}
	})

	t.Run("error on pointer to non-struct", func(t *testing.T) {
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var name string
		err := Unmarshal(re, "Alice", &name) // Pointer to string, not struct
		if err == nil {
			t.Error("Unmarshal() expected error for pointer to non-struct, got nil")
		}
	})

	t.Run("bool type conversion", func(t *testing.T) {
		type Config struct {
			Enabled string
			Debug   bool
		}
		re := regexp.MustCompile(`enabled=(?P<enabled>\w+) debug=(?P<debug>\w+)`)
		var config Config
		err := Unmarshal(re, "enabled=yes debug=true", &config)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if config.Enabled != "yes" {
			t.Errorf("Enabled = %q, want %q", config.Enabled, "yes")
		}
		if config.Debug != true {
			t.Errorf("Debug = %v, want %v", config.Debug, true)
		}
	})

	t.Run("skip unexported fields", func(t *testing.T) {
		type Data struct {
			Public  string
			private string
		}
		re := regexp.MustCompile(`(?P<public>\w+) (?P<private>\w+)`)
		var data Data
		err := Unmarshal(re, "hello world", &data)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if data.Public != "hello" {
			t.Errorf("Public = %q, want %q", data.Public, "hello")
		}
		if data.private != "" {
			t.Errorf("private should be empty, got %q", data.private)
		}
	})

	t.Run("struct tag takes precedence over field name", func(t *testing.T) {
		type Data struct {
			// Field name is "Value" but tag maps to "count"
			Value string `regex:"count"`
		}
		// Pattern has both "value" and "count" groups
		re := regexp.MustCompile(`value=(?P<value>\w+) count=(?P<count>\d+)`)
		var data Data
		err := Unmarshal(re, "value=hello count=42", &data)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		// Should get "42" from "count" group (via tag), not "hello" from "value" group (field name)
		if data.Value != "42" {
			t.Errorf("Value = %q, want %q (should use tag mapping)", data.Value, "42")
		}
	})
}

func ExampleUnmarshal() {
	type Person struct {
		Name string
		Age  int
	}

	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+) years old`)
	var person Person
	err := Unmarshal(re, "Alice is 30 years old", &person)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("%s is %d\n", person.Name, person.Age)
	// Output: Alice is 30
}

func ExampleUnmarshal_structTags() {
	type Email struct {
		Username string `regex:"user"`
		Domain   string `regex:"domain"`
	}

	re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
	var email Email
	err := Unmarshal(re, "alice@example.com", &email)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("User: %s, Domain: %s\n", email.Username, email.Domain)
	// Output: User: alice, Domain: example.com
}

func TestUnmarshalAll(t *testing.T) {
	t.Run("multiple matches", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
		var people []Person
		err := UnmarshalAll(re, "Alice is 30 and Bob is 25", &people)
		if err != nil {
			t.Fatalf("UnmarshalAll() error = %v", err)
		}
		if len(people) != 2 {
			t.Fatalf("len(people) = %d, want 2", len(people))
		}
		if people[0].Name != "Alice" || people[0].Age != 30 {
			t.Errorf("people[0] = %+v, want {Name:Alice Age:30}", people[0])
		}
		if people[1].Name != "Bob" || people[1].Age != 25 {
			t.Errorf("people[1] = %+v, want {Name:Bob Age:25}", people[1])
		}
	})

	t.Run("no matches clears slice", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>[a-z]+)`)
		people := []Person{{Name: "existing"}}
		err := UnmarshalAll(re, "123", &people)
		if err != nil {
			t.Fatalf("UnmarshalAll() error = %v", err)
		}
		if len(people) != 0 {
			t.Errorf("len(people) = %d, want 0 (should clear slice)", len(people))
		}
	})

	t.Run("single match", func(t *testing.T) {
		type Email struct {
			User   string `regex:"user"`
			Domain string `regex:"domain"`
		}
		re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
		var emails []Email
		err := UnmarshalAll(re, "Contact: alice@example.com", &emails)
		if err != nil {
			t.Fatalf("UnmarshalAll() error = %v", err)
		}
		if len(emails) != 1 {
			t.Fatalf("len(emails) = %d, want 1", len(emails))
		}
		if emails[0].User != "alice" || emails[0].Domain != "example.com" {
			t.Errorf("emails[0] = %+v, want {User:alice Domain:example.com}", emails[0])
		}
	})

	t.Run("three matches with different types", func(t *testing.T) {
		type Product struct {
			Name  string
			Price float64
		}
		re := regexp.MustCompile(`(?P<name>\w+):\$(?P<price>[\d.]+)`)
		var products []Product
		err := UnmarshalAll(re, "Items: Apple:$1.50 Banana:$0.75 Orange:$2.00", &products)
		if err != nil {
			t.Fatalf("UnmarshalAll() error = %v", err)
		}
		if len(products) != 3 {
			t.Fatalf("len(products) = %d, want 3", len(products))
		}
		want := []Product{
			{Name: "Apple", Price: 1.50},
			{Name: "Banana", Price: 0.75},
			{Name: "Orange", Price: 2.00},
		}
		if !reflect.DeepEqual(products, want) {
			t.Errorf("products = %+v, want %+v", products, want)
		}
	})

	t.Run("error on non-pointer", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var people []Person
		err := UnmarshalAll(re, "Alice", people) // Not a pointer
		if err == nil {
			t.Error("UnmarshalAll() expected error for non-pointer, got nil")
		}
	})

	t.Run("error on nil pointer", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var people *[]Person
		err := UnmarshalAll(re, "Alice", people) // Nil pointer
		if err == nil {
			t.Error("UnmarshalAll() expected error for nil pointer, got nil")
		}
	})

	t.Run("error on pointer to non-slice", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var person Person
		err := UnmarshalAll(re, "Alice", &person) // Pointer to struct, not slice
		if err == nil {
			t.Error("UnmarshalAll() expected error for pointer to non-slice, got nil")
		}
	})

	t.Run("error on slice of non-structs", func(t *testing.T) {
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var names []string
		err := UnmarshalAll(re, "Alice", &names) // Slice of strings, not structs
		if err == nil {
			t.Error("UnmarshalAll() expected error for slice of non-structs, got nil")
		}
	})
}

func ExampleUnmarshalAll() {
	type Person struct {
		Name string
		Age  int
	}

	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
	var people []Person
	err := UnmarshalAll(re, "Alice is 30 and Bob is 25", &people)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	for _, person := range people {
		fmt.Printf("%s: %d\n", person.Name, person.Age)
	}
	// Output: Alice: 30
	// Bob: 25
}
