package collection

import "encoding/json"

/*
ToJSON converts the collection's items into a compact JSON string.

Example:
	products := collection.New([]Product{
		{Name: "Desk", Price: 200},
		{Name: "Chair", Price: 100},
	})

	jsonStr, _ := products.ToJSON()

Result:
	[
	  {"name":"Desk","price":200},
	  {"name":"Chair","price":100}
	]

Notes:
  • The JSON is returned as a compact one-line string (no indentation).
  • The method never mutates the collection.
*/
func (c Collection[T]) ToJSON() (string, error) {
	b, err := json.Marshal(c.items)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

/*
ToPrettyJSON converts the collection's items into a human-readable,
formatted JSON string using indentation.

Example:
	products := collection.New([]Product{
		{Name: "Desk", Price: 200},
		{Name: "Chair", Price: 100},
	})

	jsonStr, _ := products.ToPrettyJSON()

Pretty Printed Result:
	[
	  {
	    "name": "Desk",
	    "price": 200
	  },
	  {
	    "name": "Chair",
	    "price": 100
	  }
	]

Notes:
  • Output uses two-space indentation.
  • The method never mutates the collection.
*/
func (c Collection[T]) ToPrettyJSON() (string, error) {
	b, err := json.MarshalIndent(c.items, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
