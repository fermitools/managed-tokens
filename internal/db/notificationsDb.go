package db

import (
	"context"
	"errors"

	_ "github.com/mattn/go-sqlite3"
	log "github.com/sirupsen/logrus"
)

// SQL statements to be used by API

// Query db actions
var (
	getSetupErrorsCountsStatement = `
	SELECT
		services.name,
		setup_errors.count
	FROM
		setup_errors
		INNER JOIN services ON services.id = setup_errors.service_id
	;
	`
	getPushErrorsCountsStatement = `
	SELECT
		services.name,
		nodes.name,
		push_errors.count
	FROM
		push_errors
		INNER JOIN services ON services.id = push_errors.service_id
		INNER JOIN nodes on nodes.id = push_errors.node_id
	;
	`
	getAllServicesFromTableStatement = `
	SELECT name FROM services;
	`
	getAllNodesFromTableStatement = `
	SELECT name FROM nodes;
	`
)

// INSERT/UPDATE actions
var (
	insertIntoServicesTableStatement = `
	INSERT INTO services(name)
	VALUES
		(?)
	ON CONFLICT(name) DO NOTHING;
	`
	insertIntoNodesTableStatement = `
	INSERT INTO nodes(name)
	VALUES
		(?)
	ON CONFLICT(name) DO NOTHING;
	`
	insertOrUpdateSetupErrorsStatement = `
	INSERT INTO setup_errors(service_id, count)
		SELECT
			services.id,
			? as new_count
		FROM
			services
		WHERE
			services.name = ?
	ON CONFLICT(service_id) DO
		UPDATE SET count = ?
	;
	`
	insertOrUpdatePushErrorsStatement = `
	INSERT INTO push_errors(service_id, node_id, count)
		SELECT
			services.id,
			nodes.id,
			? as new_count
		FROM
			push_errors
			INNER JOIN services ON services.id = push_errors.service_id
			INNER JOIN nodes ON nodes.id = push_errors.node_id
		WHERE
			services.name = ?
			AND nodes.name = ?
	ON CONFLICT(service_id, node_id) DO
		UPDATE SET count = ?
	;
	`
)

// External-facing functions to modify db
// TODO Make sure all funcs are documented
// GetAllServices queries the NotificationsDatabase for the registered services
// and returns a slice of strings with their names
func (m *ManagedTokensDatabase) GetAllServices(ctx context.Context) ([]string, error) {
	dataConverted := make([]string, 0)
	data, err := getValuesTransactionRunner(ctx, m.db, getAllServicesFromTableStatement)
	if err != nil {
		log.Error("Could not get service names from notifications database")
		return dataConverted, err
	}

	if len(data) == 0 {
		log.Debug("No services in database")
		return dataConverted, nil
	}

	for _, resultRow := range data {
		if len(resultRow) != 1 {
			msg := "service name data has wrong structure"
			log.Errorf("%s: %v", msg, resultRow)
			return dataConverted, errors.New(msg)
		}
		if val, ok := resultRow[0].(string); !ok {
			msg := "service name query result has wrong type.  Expected string"
			log.Errorf("%s: %T", msg, val)
			return dataConverted, errors.New(msg)
		} else {
			dataConverted = append(dataConverted, val)
		}
	}
	return dataConverted, nil
}

// GetAllNodes queries the NotificationsDatabase for the registered nodes
// and returns a slice of strings with their names
func (m *ManagedTokensDatabase) GetAllNodes(ctx context.Context) ([]string, error) {
	dataConverted := make([]string, 0)
	data, err := getValuesTransactionRunner(ctx, m.db, getAllNodesFromTableStatement)
	if err != nil {
		log.Error("Could not get node names from notifications database")
		return dataConverted, err
	}

	if len(data) == 0 {
		log.Debug("No nodes in database")
		return dataConverted, nil
	}

	for _, resultRow := range data {
		if len(resultRow) != 1 {
			msg := "node name data has wrong structure"
			log.Errorf("%s: %v", msg, resultRow)
			return dataConverted, errors.New(msg)
		}
		if val, ok := resultRow[0].(string); !ok {
			msg := "node name query result has wrong type.  Expected string"
			log.Errorf("%s: got %T", msg, val)
			return dataConverted, errors.New(msg)
		} else {
			dataConverted = append(dataConverted, val)
		}
	}
	return dataConverted, nil
}

type SetupErrorCount interface {
	Service() string
	Count() int
}

// Implements both SetupErrorCount and insertData
type setupErrorCount struct {
	service string
	count   int
}

func (s *setupErrorCount) Service() string { return s.service }
func (s *setupErrorCount) Count() int      { return s.count }
func (s *setupErrorCount) values() []any   { return []any{s.service, s.count} }

func (m *ManagedTokensDatabase) GetSetupErrorsInfo(ctx context.Context) ([]SetupErrorCount, error) {
	dataConverted := make([]SetupErrorCount, 0)
	data, err := getValuesTransactionRunner(ctx, m.db, getSetupErrorsCountsStatement)
	if err != nil {
		log.Error("Could not get setup errors information from notifications database")
		return dataConverted, err
	}

	if len(data) == 0 {
		log.Debug("No setup error data in database")
		return dataConverted, nil
	}

	// Unpack data
	for _, resultRow := range data {
		// Make sure we have the right number of values
		if len(resultRow) != 2 {
			msg := "setup error data has wrong structure"
			log.Errorf("%s: %v", msg, resultRow)
			return dataConverted, errors.New(msg)
		}
		// Type check each element
		serviceVal, serviceTypeOk := resultRow[0].(string)
		countVal, countTypeOk := resultRow[1].(int64)
		if !(serviceTypeOk && countTypeOk) {
			msg := "setup errors query result has wrong type.  Expected (string, int)"
			log.Errorf("%s: got (%T, %T)", msg, serviceVal, countVal)
			return dataConverted, errors.New(msg)
		}
		dataConverted = append(dataConverted, &setupErrorCount{serviceVal, int(countVal)})
	}
	return dataConverted, nil
}

type PushErrorCount interface {
	Service() string
	Node() string
	Count() int
}

// Implements both PushErrorCount and insertData
type pushErrorCount struct {
	service string
	node    string
	count   int
}

func (s *pushErrorCount) Service() string { return s.service }
func (s *pushErrorCount) Node() string    { return s.node }
func (s *pushErrorCount) Count() int      { return s.count }
func (p *pushErrorCount) values() []any   { return []any{p.service, p.node, p.count} }

func (m *ManagedTokensDatabase) GetPushErrorsInfo(ctx context.Context) ([]PushErrorCount, error) {
	dataConverted := make([]PushErrorCount, 0)
	data, err := getValuesTransactionRunner(ctx, m.db, getPushErrorsCountsStatement)
	if err != nil {
		log.Error("Could not get push errors information from notifications database")
		return dataConverted, err
	}

	if len(data) == 0 {
		log.Debug("No push error data in database")
		return dataConverted, nil
	}

	// Unpack data
	for _, resultRow := range data {
		if len(resultRow) != 3 {
			msg := "push error data has wrong structure"
			log.Errorf("%s: %v", msg, resultRow)
			return dataConverted, errors.New(msg)
		}
		// Type check each element
		serviceVal, serviceTypeOk := resultRow[0].(string)
		nodeVal, nodeTypeOk := resultRow[1].(string)
		countVal, countTypeOk := resultRow[2].(int64)
		if !(serviceTypeOk && nodeTypeOk && countTypeOk) {
			msg := "push errors query result has wrong type.  Expected (string, string, int)"
			log.Errorf("%s: got (%T, %T, %T)", msg, serviceVal, nodeVal, countVal)
			return dataConverted, errors.New(msg)
		}
		dataConverted = append(dataConverted, &pushErrorCount{serviceVal, nodeVal, int(countVal)})
	}
	return dataConverted, nil
}

type serviceDatum struct{ value string }

func (s *serviceDatum) values() []any { return []any{s.value} }

// UpdateServices updates the services table in the NotificationsDatabase.  It takes a slice
// of strings for the service names, and inserts them if they don't already exist in the
// database
func (m *ManagedTokensDatabase) UpdateServices(ctx context.Context, serviceNames []string) error {
	serviceDatumSlice := make([]insertValues, len(serviceNames))
	for _, s := range serviceNames {
		serviceDatumSlice = append(serviceDatumSlice, &serviceDatum{s})
	}

	if err := insertTransactionRunner(ctx, m.db, insertIntoServicesTableStatement, serviceDatumSlice); err != nil {
		log.Error("Could not update services in notifications database")
		return err
	}
	log.Debug("Updated services in notifications database")
	return nil
}

type nodeDatum struct{ value string }

func (n *nodeDatum) values() []any { return []any{n.value} }

// UpdateNodes updates the nodes table in the NotificationsDatabase.  It takes a slice
// of strings for the node names, and inserts them if they don't already exist in the
// database
func (m *ManagedTokensDatabase) UpdateNodes(ctx context.Context, nodes []string) error {
	nodesDatumSlice := make([]insertValues, len(nodes))
	for _, s := range nodes {
		nodesDatumSlice = append(nodesDatumSlice, &nodeDatum{s})
	}

	if err := insertTransactionRunner(ctx, m.db, insertIntoNodesTableStatement, nodesDatumSlice); err != nil {
		log.Error("Could not update nodes in notifications database")
		return err
	}
	log.Debug("Updated nodes in notifications database")
	return nil

}

func (m *ManagedTokensDatabase) UpdateSetupErrorsTable(ctx context.Context, setupErrorsByService []SetupErrorCount) error {
	setupErrorDatumSlice := make([]insertValues, 0)
	for _, datum := range setupErrorsByService {
		setupErrorDatumSlice = append(setupErrorDatumSlice,
			&setupErrorCount{
				service: datum.Service(),
				count:   datum.Count(),
			})
	}

	if err := insertTransactionRunner(ctx, m.db, insertOrUpdateSetupErrorsStatement, setupErrorDatumSlice); err != nil {
		log.Error("Could not update setup errors in notifications database")
		return err
	}
	log.Debug("Updated setup errors in notifications database")
	return nil

}

func (m *ManagedTokensDatabase) UpdatePushErrorsTable(ctx context.Context, pushErrorsByServiceAndNode []PushErrorCount) error {
	pushErrorDatumSlice := make([]insertValues, 0)
	for _, datum := range pushErrorsByServiceAndNode {
		pushErrorDatumSlice = append(pushErrorDatumSlice,
			&pushErrorCount{
				service: datum.Service(),
				node:    datum.Node(),
				count:   datum.Count(),
			})
	}

	if err := insertTransactionRunner(ctx, m.db, insertOrUpdatePushErrorsStatement, pushErrorDatumSlice); err != nil {
		log.Error("Could not update push errors in notifications database")
		return err
	}
	log.Debug("Updated push errors in notifications database")
	return nil
}

// Pragma:  PRAGMA foreign_keys = ON;
// Schema:
// sqlite> .schema
// CREATE TABLE services (
// id INTEGER NOT NULL PRIMARY KEY,
// name STRING NOT NULL
// );
// CREATE TABLE nodes (
// id INTEGER NOT NULL PRIMARY KEY,
// name STRING NOT NULL
// );
// CREATE TABLE push_errors (
// service_id INTEGER,
// node_id INTEGER,
// count INTEGER,
// FOREIGN KEY (service_id)
//   REFERENCES services (id)
//     ON DELETE CASCADE
//     ON UPDATE NO ACTION,
// FOREIGN KEY (node_id)
//   REFERENCES nodes (id)
//     ON DELETE CASCADE
//     ON UPDATE NO ACTION
// );
// CREATE TABLE setup_errors (
// service_id INTEGER,
// count INTEGER,
// FOREIGN KEY (service_id)
//   REFERENCES services (id)
//     ON DELETE CASCADE
//     ON UPDATE NO ACTION
// );

// // OpenOrCreateNotificationsDatabase opens a sqlite3 database for reading or writing, and returns a *NotificationsDatabase object.  If the database already
// // exists at the filename provided, it will open that database as long as the ApplicationId matches
// func OpenOrCreateNotificationsDatabase(filename string) (*NotificationsDatabase, error) {
// 	var err error
// 	n := NotificationsDatabase{filename: filename}
// 	err = openOrCreateDatabase(&n)
// 	if err != nil {
// 		log.WithField("filename", n.filename).Error("Could not create or open NotificationsDatabase")
// 	}
// 	return &n, err
// }

// // Filename returns the path to the file holding the database
// func (n *NotificationsDatabase) Filename() string {
// 	return n.filename
// }

// // Database returns the underlying SQL database of the NotificationsDatabase
// func (n *NotificationsDatabase) Database() *sql.DB {
// 	return n.db
// }

// // Open opens the database located at NotificationsDatabase.filename and stores the opened *sql.DB object in the NotificationsDatabase
// func (n *NotificationsDatabase) Open() error {
// 	var err error
// 	n.db, err = sql.Open("sqlite3", n.filename)
// 	if err != nil {
// 		msg := "Could not open the database file"
// 		log.WithField("filename", n.filename).Errorf("%s: %s", msg, err)
// 		return &databaseOpenError{msg}
// 	}
// 	// Enforce foreign key constraints
// 	if _, err := n.db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
// 		log.WithField("filename", n.filename).Error(err)
// 		return err
// 	}
// 	return nil
// }

// // Close closes the NotificationsDatabase
// func (n *NotificationsDatabase) Close() error {
// 	return n.db.Close()
// }

// // initialize prepares a new NotificationsDatabase for use and returns a pointer to the underlying sql.DB
// func (n *NotificationsDatabase) initialize() error {
// 	var err error
// 	if err = n.Open(); err != nil {
// 		log.WithField("filename", n.filename).Error(err)
// 		return err
// 	}

// 	// Set our application ID
// 	if _, err := n.db.Exec(fmt.Sprintf("PRAGMA application_id=%d;", ApplicationId)); err != nil {
// 		log.WithField("filename", n.filename).Error(err)
// 		return err
// }

// 	// Create the tables in the database
// 	if err := n.createServicesTable(); err != nil {
// 		log.WithField("filename", n.filename).Error(err)
// 		return &databaseCreateError{err.Error()}
// 	}
// 	if err := n.createNodesTable(); err != nil {
// 		log.WithField("filename", n.filename).Error(err)
// 		return &databaseCreateError{err.Error()}
// 	}
// 	if err := n.createSetupErrorsTable(); err != nil {
// 		log.WithField("filename", n.filename).Error(err)
// 		return &databaseCreateError{err.Error()}
// 	}
// 	if err := n.createPushErrorsTable(); err != nil {
// 		log.WithField("filename", n.filename).Error(err)
// 		return &databaseCreateError{err.Error()}
// 	}

// 	return nil
// }

// // NotificationsDatabaseCountDatum is a piece of data that can be used to hold information about
// // notifications counts for a given service and optionally a node
// type NotificationsDatabaseCountDatum interface {
// 	ServiceName() string
// 	NodeName() string
// 	Count() uint16
// 	String() string
// }

// // // NotificationsDatabase is a database in which information about how many notifications have been sent about a particular failure have
// // // been generated
// // type NotificationsDatabase struct {
// // 	filename string
// // 	db       *sql.DB
// // }