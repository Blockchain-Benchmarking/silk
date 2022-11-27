package kv


// ----------------------------------------------------------------------------


type Table interface {
	View

	Set(Key, Value) Value
}

func NewTable() Table {
	return newTable()
}

func NewTableFrom(keys []Key, values []Value) Table {
	return newTableFrom(keys, values)
}


type View interface {
	Get(Key) Value

	List() []Key

	Snapshot() View
}


// ----------------------------------------------------------------------------


type table struct {
	values map[string]Value
}

func newTable() *table {
	var this table

	this.values = make(map[string]Value)

	return &this
}

func newTableFrom(keys []Key, values []Value) *table {
	var this table
	var i int

	if len(keys) != len(values) {
		panic("keys and values sizes differ")
	}

	this.values = make(map[string]Value, len(keys))

	for i = range keys {
		this.values[keys[i].String()] = values[i]
	}

	return &this
}

func (this *table) Get(key Key) Value {
	return newView(this.values).Get(key)
}

func (this *table) List() []Key {
	return newView(this.values).List()
}

func (this *table) Set(key Key, value Value) Value {
	var ks string = key.String()
	var found bool
	var old Value

	old, found = this.values[ks]

	if value == NoValue {
		if found {
			delete(this.values, ks)
		}
	} else {
		this.values[ks] = value
	}

	if found {
		return old
	} else {
		return NoValue
	}
}

func (this *table) Snapshot() View {
	var cp map[string]Value
	var ks string
	var val Value

	cp = make(map[string]Value, len(this.values))

	for ks, val = range this.values {
		cp[ks] = val
	}

	return newView(cp)
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type view struct {
	values map[string]Value
}

func newView(values map[string]Value) *view {
	var this view

	this.values = values

	return &this
}

func (this *view) Get(key Key) Value {
	var ks string = key.String()
	var val Value
	var found bool

	val, found = this.values[ks]

	if found {
		return val
	} else {
		return NoValue
	}
}

func (this *view) List() []Key {
	var keys []Key = make([]Key, 0, len(this.values))
	var ks string

	for ks = range this.values {
		keys = append(keys, &key{ ks })
	}

	return keys
}

func (this *view) Snapshot() View {
	return this
}
