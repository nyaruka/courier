package batch

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
)

type Label struct {
	ID    int    `db:"id"`
	Label string `db:"label"`
}

func (l *Label) RowID() string {
	if l.ID != 0 {
		return fmt.Sprintf("%d", l.ID)
	}
	return ""
}

func TestBatchInsert(t *testing.T) {
	db := sqlx.MustConnect("postgres", "postgres://courier@localhost/courier_test?sslmode=disable")
	db.MustExec("DROP TABLE IF EXISTS labels;")
	db.MustExec("CREATE TABLE labels(id serial primary key, label text not null unique);")

	var callbackErr error
	callback := func(err error, value Value) {
		callbackErr = err
	}

	committer := NewCommitter("labels", db, "INSERT INTO labels(label) VALUES(:label);", time.Millisecond*250, &sync.WaitGroup{}, callback)
	committer.Start()
	defer committer.Stop()

	committer.Queue(&Label{0, "label1"})
	committer.Queue(&Label{0, "label2"})
	committer.Queue(&Label{0, "label3"})

	time.Sleep(time.Second)

	assert.NoError(t, callbackErr)
	count := 0
	db.Get(&count, "SELECT count(*) FROM labels;")
	assert.Equal(t, 3, count)

	committer.Queue(&Label{0, "label4"})
	committer.Queue(&Label{0, "label3"})

	time.Sleep(time.Second)

	assert.Error(t, callbackErr)
	assert.Equal(t, `labels: error comitting value: error during bulk insert: pq: duplicate key value violates unique constraint "labels_label_key"`, callbackErr.Error())
	db.Get(&count, "SELECT count(*) FROM labels;")
	assert.Equal(t, 4, count)
}

func TestBatchUpdate(t *testing.T) {
	db := sqlx.MustConnect("postgres", "postgres://courier@localhost/courier_test?sslmode=disable")
	db.MustExec("DROP TABLE IF EXISTS labels;")
	db.MustExec("CREATE TABLE labels(id serial primary key, label text not null unique);")
	db.MustExec("INSERT INTO labels(label) VALUES('label1'), ('label2'), ('label3');")

	var callbackErr error
	callback := func(err error, value Value) {
		callbackErr = err
	}

	committer := NewCommitter("labels", db, `
	UPDATE 
	  labels 
	SET 
	  label = l.status
	FROM 
	  (VALUES(:id, :label)) 
	AS 
	  l(id, status) 
	WHERE 
	  labels.id = l.id::int;
	`, time.Millisecond*250, &sync.WaitGroup{}, callback)

	committer.Queue(&Label{1, "label01"})
	committer.Queue(&Label{2, "label02"})
	committer.Queue(&Label{1, "label001"})
	committer.Queue(&Label{3, "label03"})

	committer.Start()
	defer committer.Stop()

	time.Sleep(time.Second)

	assert.NoError(t, callbackErr)
	count := 0
	db.Get(&count, "SELECT count(*) FROM labels;")
	assert.Equal(t, 3, count)

	label := ""
	db.Get(&label, "SELECT label FROM labels WHERE id = 1;")
	assert.Equal(t, "label001", label)

	db.Get(&label, "SELECT label FROM labels WHERE id = 2;")
	assert.Equal(t, "label02", label)

	db.Get(&label, "SELECT label FROM labels WHERE id = 3;")
	assert.Equal(t, "label03", label)
}
