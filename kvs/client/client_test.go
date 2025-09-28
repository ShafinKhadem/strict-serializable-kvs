package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

var hosts = []string{"localhost:8080"}

func TestNop(t *testing.T) {
	client := Dial(hosts[0])
	client.Begin()
	client.Commit()
}

func (client *Client) PutTx(k string) {
	client.Begin()
	err := client.Put(k, k)
	if err != nil {
		panic("put failed")
	}
	client.Commit()
}

func (client *Client) GetTx(k string) string {
	client.Begin()
	value, err := client.Get(k)
	if err != nil {
		panic("put failed")
	}
	client.Commit()
	return value
}

func TestPutGet(t *testing.T) {
	// client := Dial(hosts[0])
	client := NewClient([]string{"localhost:8080"})

	// tx0
	client.Begin()

	err := client.Put("test", "value")
	assert.Nil(t, err)

	got, err := client.Get("test")
	assert.Nil(t, err)
	assert.Equal(t, "value", got)

	client.Commit()

	// tx1
	client.Begin()
	got, err = client.Get("test")
	assert.Nil(t, err)
	client.Commit()

	assert.Equal(t, "value", got)

	for i := 0; i < 1024; i++ {
		is := fmt.Sprintf("%v", i)
		client.PutTx(is)
		r := client.GetTx(is)
		assert.Equal(t, is, r)
	}

	for i := 0; i < 1024; i++ {
		is := fmt.Sprintf("%v", i)
		client.PutTx(is)
	}

	for i := 0; i < 1024; i++ {
		is := fmt.Sprintf("%v", i)
		r := client.GetTx(is)
		assert.Equal(t, is, r)
	}
}

func TestWWConflict(t *testing.T) {
	// c1 := Dial(hosts[0])
	// c2 := Dial(hosts[0])

	c1 := NewClient([]string{"localhost:8080"})
    c2 := NewClient([]string{"localhost:8080"})

	c1.Begin()
	c2.Begin()

	err := c1.Put("c1", "c1")
	assert.Nil(t, err)

	err = c2.Put("c2", "c2")
	assert.Nil(t, err)

	c1.Commit()
	c2.Commit()

	c1.Begin()
	c2.Begin()

	err = c1.Put("c1", "c1")
	assert.Nil(t, err)

	err = c2.Put("c1", "c2")
	assert.NotNil(t, err)

	c1.Commit()
	assert.Panics(t, func() { c2.Commit() }, "c2 commit should fail")
}

func TestRWConflict(t *testing.T) {
	// c1 := Dial(hosts[0])
	// c2 := Dial(hosts[0])

	c1 := NewClient([]string{"localhost:8080"})
    c2 := NewClient([]string{"localhost:8080"})

	c1.Begin()
	c2.Begin()

	err := c1.Put("c1", "c1")
	assert.Nil(t, err)

	_, err = c2.Get("c2")
	assert.Nil(t, err)

	c1.Commit()
	c2.Commit()

	c1.Begin()
	c2.Begin()

	got, err := c1.Get("c1")
	assert.Nil(t, err)
	assert.Equal(t, "c1", got)

	err = c2.Put("c1", "c2")
	assert.NotNil(t, err)

	c1.Commit()
	assert.Panics(t, func() { c2.Commit() }, "c2 commit should fail")

	c1.Begin()
	c2.Begin()

	err = c1.Put("c1", "c3")
	assert.Nil(t, err)

	_, err = c2.Get("c1")
	assert.NotNil(t, err)

	c1.Commit()
	assert.Panics(t, func() { c2.Commit() }, "c2 commit should fail")
}
