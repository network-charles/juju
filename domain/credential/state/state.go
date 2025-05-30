// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/changestream"
	corecredential "github.com/juju/juju/core/credential"
	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	userstate "github.com/juju/juju/domain/access/state"
	"github.com/juju/juju/domain/credential"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// State is used to access the database.
type State struct {
	*domain.StateBase
}

// NewState creates a state to access the database.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// CredentialUUIDForKey finds and returns the uuid for the cloud credential
// identified by key. If no credential is found then an error of
// [credentialerrors.NotFound] is returned.
func (st *State) CredentialUUIDForKey(ctx context.Context, key corecredential.Key) (corecredential.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return corecredential.UUID(""), errors.Capture(err)
	}

	var rval corecredential.UUID
	return rval, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		rval, err = st.credentialUUIDForKey(ctx, tx, key)
		return err
	})
}

// credentialUUIDForKey finds and returns the uuid for the cloud credential
// identified by key. If no credential is found then an error of
// [credentialerrors.NotFound] is returned.
func (st *State) credentialUUIDForKey(
	ctx context.Context,
	tx *sqlair.TX,
	key corecredential.Key,
) (corecredential.UUID, error) {
	dbKey := credentialKey{
		CredentialName: key.Name,
		CloudName:      key.Cloud,
		OwnerName:      key.Owner.String(),
	}
	result := credentialUUID{}

	selectStmt, err := st.Prepare(`
SELECT &credentialUUID.uuid
FROM   v_cloud_credential
WHERE  name = $credentialKey.name
AND    owner_name = $credentialKey.owner_name
AND    cloud_name = $credentialKey.cloud_name
`, dbKey, result)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, selectStmt, dbKey).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.Errorf("cloud credential %q %w", key, credentialerrors.NotFound)
	} else if err != nil {
		return "", errors.Errorf("fetching cloud credential %q: %w", key, err)
	}
	return corecredential.UUID(result.UUID), nil
}

// UpsertCloudCredential adds or updates a cloud credential with the given name,
// cloud and owner.
//
// If the owner of the credential can't be found then an error satisfying
// [usererrors.NotFound] will be returned.
func (st *State) UpsertCloudCredential(ctx context.Context, key corecredential.Key, credential credential.CloudCredentialInfo) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	dbKey := credentialKey{
		CredentialName: key.Name,
		CloudName:      key.Cloud,
		OwnerName:      key.Owner.String(),
	}
	stmt, err := st.Prepare(`
SELECT uuid AS &credentialUUID.uuid
FROM   v_cloud_credential
WHERE  name = $credentialKey.name
AND    owner_name = $credentialKey.owner_name
AND    cloud_name = $credentialKey.cloud_name
`, dbKey, credentialUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get the credential UUID - either existing or make a new one.
		// TODO(wallyworld) - implement owner as a FK when users are modelled.

		result := credentialUUID{}
		err = tx.Query(ctx, stmt, dbKey).Get(&result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		} else if err != nil {
			if credential.Invalid || credential.InvalidReason != "" {
				return errors.Errorf("adding invalid credential %w", coreerrors.NotSupported)
			}
			id, err := corecredential.NewUUID()
			if err != nil {
				return errors.Errorf("generating new credential uuid: %w", err)
			}
			result.UUID = id.String()
		}

		if err := upsertCredential(ctx, tx, result.UUID, key, credential); err != nil {
			return errors.Errorf("updating credential: %w", err)
		}

		if err := updateCredentialAttributes(ctx, tx, result.UUID, credential.Attributes); err != nil {
			return errors.Errorf("updating credential %q attributes: %w", key.Name, err)
		}
		return nil
	})

	return errors.Capture(err)
}

// CreateCredential saves the specified credential.
// Exported for use in the related credential bootstrap package.
func CreateCredential(ctx context.Context, tx *sqlair.TX, credentialUUID string, key corecredential.Key, credential credential.CloudCredentialInfo) error {
	if err := upsertCredential(ctx, tx, credentialUUID, key, credential); err != nil {
		return errors.Errorf("creating credential %s: %w", credentialUUID, err)
	}
	if err := updateCredentialAttributes(ctx, tx, credentialUUID, credential.Attributes); err != nil {
		return errors.Errorf("creating credential %s attributes: %w", credentialUUID, err)
	}
	return nil
}

func upsertCredential(ctx context.Context, tx *sqlair.TX, credentialUUID string, key corecredential.Key, credential credential.CloudCredentialInfo) error {
	dbCredential, err := dbCredentialFromCredential(ctx, tx, credentialUUID, key, credential)
	if err != nil {
		return errors.Capture(err)
	}

	insertQuery := `
INSERT INTO cloud_credential (uuid, name, cloud_uuid, auth_type_id, owner_uuid, revoked, invalid, invalid_reason)
VALUES (
    $Credential.uuid,
    $Credential.name,
    $Credential.cloud_uuid,
    $Credential.auth_type_id,
    $Credential.owner_uuid,
    $Credential.revoked,
    $Credential.invalid,
    $Credential.invalid_reason
)
ON CONFLICT(uuid) DO UPDATE SET name=excluded.name,
                                cloud_uuid=excluded.cloud_uuid,
                                auth_type_id=excluded.auth_type_id,
                                owner_uuid=excluded.owner_uuid,
                                revoked=excluded.revoked,
                                invalid=excluded.invalid,
                                invalid_reason=excluded.invalid_reason
`

	insertStmt, err := sqlair.Prepare(insertQuery, Credential{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertStmt, dbCredential).Run()
	if database.IsErrConstraintCheck(err) {
		return errors.Errorf("credential name cannot be empty").Add(coreerrors.NotValid)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

func updateCredentialAttributes(ctx context.Context, tx *sqlair.TX, credentialUUID string, attr credentialAttrs) error {
	// Delete any keys no longer in the attributes map.
	// TODO(wallyworld) - sqlair does not support IN operations with a list of values
	deleteQuery := `
DELETE FROM  cloud_credential_attribute
WHERE        cloud_credential_uuid = $M.uuid
`

	deleteStmt, err := sqlair.Prepare(deleteQuery, sqlair.M{})
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteStmt, sqlair.M{"uuid": credentialUUID}).Run(); err != nil {
		return errors.Capture(err)
	}

	insertQuery := `
INSERT INTO cloud_credential_attribute
VALUES (
    $CredentialAttribute.cloud_credential_uuid,
    $CredentialAttribute.key,
    $CredentialAttribute.value
)
ON CONFLICT(cloud_credential_uuid, key) DO UPDATE SET key=excluded.key,
                                                      value=excluded.value
`
	insertStmt, err := sqlair.Prepare(insertQuery, CredentialAttribute{})
	if err != nil {
		return errors.Capture(err)
	}

	for key, value := range attr {
		if err := tx.Query(ctx, insertStmt, CredentialAttribute{
			CredentialUUID: credentialUUID,
			Key:            key,
			Value:          value,
		}).Run(); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

// dbCredentialFromCredential is responsible for populating a database
// representation of a cloud credential from a credential id and info structures.
//
// If no user is found for the credential owner then an error satisfying
// [usererrors.NotFound] will be returned.
func dbCredentialFromCredential(ctx context.Context, tx *sqlair.TX, credentialUUID string, key corecredential.Key, credential credential.CloudCredentialInfo) (*Credential, error) {
	cred := &Credential{
		ID:            credentialUUID,
		Name:          key.Name,
		AuthTypeID:    -1,
		Revoked:       credential.Revoked,
		Invalid:       credential.Invalid,
		InvalidReason: credential.InvalidReason,
	}

	userUUID, err := userstate.GetUserUUIDByName(ctx, tx, key.Owner)
	if err != nil {
		return nil, errors.Errorf("getting user uuid for credential owner %q: %w", key.Owner, err)
	}
	cred.OwnerUUID = userUUID.String()

	q := "SELECT uuid AS &Credential.cloud_uuid FROM cloud WHERE name = $dbCloudName.name"
	stmt, err := sqlair.Prepare(q, Credential{}, dbCloudName{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, dbCloudName{Name: key.Cloud}).Get(cred)
	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil, errors.Errorf("cloud %q for credential %w", key.Cloud, coreerrors.NotFound)
		}
		return nil, errors.Capture(err)
	}

	validAuthTypes, err := validAuthTypesForCloud(ctx, tx, key.Cloud)
	if err != nil {
		return nil, errors.Errorf("loading cloud auth types: %w", err)
	}
	if errors.Is(err, sqlair.ErrNoRows) {
		if err != nil {
			return nil, errors.Errorf("no valid cloud auth types: %w", err)
		}
		return nil, nil
	}
	var validAuthTypeNames []string
	for _, at := range validAuthTypes {
		if at.Type == credential.AuthType {
			cred.AuthTypeID = at.ID
		}
		validAuthTypeNames = append(validAuthTypeNames, at.Type)
	}
	if cred.AuthTypeID == -1 {
		return nil, errors.Errorf(
			"validating credential %q owned by %q for cloud %q: supported auth-types %q, %q %w",
			key.Name, key.Owner, key.Cloud, validAuthTypeNames, credential.AuthType, coreerrors.NotSupported)

	}
	return cred, nil
}

func validAuthTypesForCloud(ctx context.Context, tx *sqlair.TX, cloudName string) (authTypes, error) {
	authTypeQuery := `
SELECT &authType.*
FROM   auth_type
JOIN   cloud_auth_type ON auth_type.id = cloud_auth_type.auth_type_id
JOIN   cloud ON cloud_auth_type.cloud_uuid = cloud.uuid
WHERE  cloud.name = $dbCloudName.name
`
	cloud := dbCloudName{Name: cloudName}
	stmt, err := sqlair.Prepare(authTypeQuery, authType{}, cloud)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result authTypes
	err = tx.Query(ctx, stmt, cloud).GetAll(&result)
	return result, errors.Capture(err)
}

// InvalidateCloudCredential marks a cloud credential with the given name, cloud and owner. as invalid.
func (st *State) InvalidateCloudCredential(ctx context.Context, key corecredential.Key, reason string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	q := `
UPDATE cloud_credential
SET    invalid = true, invalid_reason = $M.invalid_reason
FROM cloud
WHERE  cloud_credential.name = $M.credential_name
AND    cloud_credential.owner_uuid = (
    SELECT uuid
    FROM user
    WHERE user.name = $M.owner
	AND user.removed = false
)
AND    cloud_credential.cloud_uuid = (
    SELECT uuid FROM cloud
    WHERE name = $M.cloud_name
)`
	stmt, err := st.Prepare(q, sqlair.M{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		terms := sqlair.M{
			"credential_name": key.Name,
			"cloud_name":      key.Cloud,
			"owner":           key.Owner.Name(),
		}
		terms["invalid_reason"] = reason
		err = tx.Query(ctx, stmt, terms).Get(&outcome)
		if err != nil {
			return errors.Capture(err)
		}
		n, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Capture(err)
		}
		if n < 1 {
			return errors.Errorf("credential %q for cloud %q owned by %q %w", key.Name, key.Cloud, key.Owner, coreerrors.NotFound)
		}
		return nil
	})
	return errors.Capture(err)
}

// CloudCredentialsForOwner returns the owner's cloud credentials for a given
// cloud, keyed by credential name.
func (st *State) CloudCredentialsForOwner(ctx context.Context, owner user.Name, cloudName string) (map[string]credential.CloudCredentialResult, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows      Credentials
		dbAuthTypes []authType
		keyValues   []CredentialAttribute
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		credQuery := `
SELECT cc.* AS &Credential.*,
       auth_type.type AS &authType.type,
       (cc_attr.key, cc_attr.value) AS (&CredentialAttribute.*)
FROM   cloud_credential cc
       JOIN auth_type ON cc.auth_type_id = auth_type.id
       JOIN cloud ON cc.cloud_uuid = cloud.uuid
	   JOIN user on cc.owner_uuid = user.uuid
       LEFT JOIN cloud_credential_attribute cc_attr ON cc_attr.cloud_credential_uuid = cc.uuid
WHERE  user.removed = false
AND    user.name = $ownerAndCloudName.owner_name
AND    cloud.name = $ownerAndCloudName.cloud_name
`
		names := ownerAndCloudName{
			OwnerName: owner.Name(),
			CloudName: cloudName,
		}
		credStmt, err := st.Prepare(
			credQuery,
			names,
			Credential{},
			authType{},
			CredentialAttribute{},
		)
		if err != nil {
			return errors.Errorf("preparing select credentials for owner statement: %w", err)
		}

		err = tx.Query(ctx, credStmt, names).GetAll(&dbRows, &dbAuthTypes, &keyValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("loading cloud credentials: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	creds, err := dbRows.ToCloudCredentials(cloudName, dbAuthTypes, keyValues)
	if err != nil {
		return nil, errors.Capture(err)
	}
	result := make(map[string]credential.CloudCredentialResult)
	for _, cred := range creds {
		result[fmt.Sprintf("%s/%s/%s", cloudName, owner, cred.Label)] = cred
	}
	return result, nil
}

// CloudCredential returns the cloud credential for the given details.
func (st *State) CloudCredential(ctx context.Context, key corecredential.Key) (credential.CloudCredentialResult, error) {
	db, err := st.DB()
	if err != nil {
		return credential.CloudCredentialResult{}, errors.Capture(err)
	}

	var (
		dbRows      Credentials
		dbAuthTypes []authType
		keyValues   []CredentialAttribute
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		credQuery := `
SELECT cc.* AS &Credential.*,
       auth_type.type AS &authType.type,
       (cc_attr.key, cc_attr.value) AS (&CredentialAttribute.*)
FROM   cloud_credential cc
       JOIN auth_type ON cc.auth_type_id = auth_type.id
       JOIN cloud ON cc.cloud_uuid = cloud.uuid
	   JOIN user on cc.owner_uuid = user.uuid
       LEFT JOIN cloud_credential_attribute cc_attr ON cc_attr.cloud_credential_uuid = cc.uuid
WHERE  user.removed = false
AND    cloud.name = $credentialKey.cloud_name
AND    user.name = $credentialKey.owner_name
AND    cc.name = $credentialKey.name
`
		credKey := credentialKey{
			CredentialName: key.Name,
			CloudName:      key.Cloud,
			OwnerName:      key.Owner.Name(),
		}
		credStmt, err := st.Prepare(
			credQuery,
			credKey,
			Credential{},
			authType{},
			CredentialAttribute{},
		)
		if err != nil {
			return errors.Capture(err)
		}

		err = tx.Query(ctx, credStmt, credKey).GetAll(&dbRows, &dbAuthTypes, &keyValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("loading cloud credentials: %w", err)
		}
		return nil
	})
	if err != nil {
		return credential.CloudCredentialResult{}, errors.Capture(err)
	}
	if len(dbRows) == 0 {
		return credential.CloudCredentialResult{}, errors.Errorf(
			"%w: credential %q for cloud %q owned by %q",
			credentialerrors.CredentialNotFound, key.Name, key.Cloud, key.Owner)

	}
	creds, err := dbRows.ToCloudCredentials(key.Cloud, dbAuthTypes, keyValues)
	if err != nil {
		return credential.CloudCredentialResult{}, errors.Capture(err)
	}
	if len(creds) > 1 {
		return credential.CloudCredentialResult{}, errors.Errorf("expected 1 credential, got %d", len(creds))
	}
	return creds[0], errors.Capture(err)
}

// GetCloudCredential is responsible for returning a cloud credential identified
// by id. If no cloud credential exists for the given id then a
// [credentialerrors.NotFound] error will be returned.
func (st *State) GetCloudCredential(
	ctx context.Context,
	id corecredential.UUID,
) (credential.CloudCredentialResult, error) {
	db, err := st.DB()
	if err != nil {
		return credential.CloudCredentialResult{}, errors.Capture(err)
	}

	var rval credential.CloudCredentialResult
	return rval, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		rval, err = GetCloudCredential(ctx, st, tx, id)
		return err
	})
}

// GetCloudCredential is responsible for returning a cloud credential identified
// by id. If no cloud credential exists for the given id then a
// [credentialerrors.NotFound] error will be returned.
func GetCloudCredential(
	ctx context.Context,
	st domain.Preparer,
	tx *sqlair.TX,
	id corecredential.UUID,
) (credential.CloudCredentialResult, error) {
	q := `
SELECT ca.* AS &credentialWithAttribute.*
FROM   v_cloud_credential_attribute ca
WHERE  uuid = $M.id
`

	stmt, err := st.Prepare(q, credentialWithAttribute{}, sqlair.M{})
	if err != nil {
		return credential.CloudCredentialResult{}, errors.Capture(err)
	}

	args := sqlair.M{
		"id": id,
	}
	rows := []credentialWithAttribute{}

	err = tx.Query(ctx, stmt, args).GetAll(&rows)
	if errors.Is(err, sql.ErrNoRows) {
		return credential.CloudCredentialResult{}, errors.Errorf("%w for id %q", credentialerrors.NotFound, id)
	} else if err != nil {
		return credential.CloudCredentialResult{}, errors.Errorf("getting cloud credential for id %q: %w", id, err)
	}

	rval := credential.CloudCredentialResult{
		CloudCredentialInfo: credential.CloudCredentialInfo{
			AuthType:      rows[0].AuthType,
			Attributes:    make(map[string]string, len(rows)),
			Revoked:       rows[0].Revoked,
			Label:         rows[0].Name,
			Invalid:       rows[0].Invalid,
			InvalidReason: rows[0].InvalidReason,
		},
		CloudName: rows[0].CloudName,
	}
	for _, row := range rows {
		rval.CloudCredentialInfo.Attributes[row.AttributeKey] = row.AttributeValue
	}
	return rval, nil
}

// AllCloudCredentialsForOwner returns all cloud credentials stored on the controller
// for a given owner.
func (st *State) AllCloudCredentialsForOwner(ctx context.Context, owner user.Name) (map[corecredential.Key]credential.CloudCredentialResult, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows       Credentials
		dbAuthTypes  []authType
		dbCloudNames []dbCloudName
		keyValues    []CredentialAttribute
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		credQuery := `
SELECT cc.* AS &Credential.*,
       auth_type.type AS &authType.type,
       cloud.name AS &dbCloudName.name,
       (cc_attr.key, cc_attr.value) AS (&CredentialAttribute.*)
FROM   cloud_credential cc
       JOIN auth_type ON cc.auth_type_id = auth_type.id
       JOIN cloud ON cc.cloud_uuid = cloud.uuid
	   JOIN user on cc.owner_uuid = user.uuid
       LEFT JOIN cloud_credential_attribute cc_attr ON cc_attr.cloud_credential_uuid = cc.uuid
WHERE  user.removed = false
AND    user.name = $ownerName.name
`
		ownerName := ownerName{
			Name: owner.Name(),
		}
		credStmt, err := st.Prepare(
			credQuery,
			ownerName,
			dbCloudName{},
			Credential{},
			authType{},
			CredentialAttribute{},
		)
		if err != nil {
			return errors.Errorf("preparing select all credentials for owner statement: %w", err)
		}

		err = tx.Query(ctx, credStmt, ownerName).GetAll(&dbRows, &dbCloudNames, &dbAuthTypes, &keyValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("loading cloud credentials: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	result := make(map[corecredential.Key]credential.CloudCredentialResult)
	for _, cloudName := range dbCloudNames {
		infos, err := dbRows.ToCloudCredentials(cloudName.Name, dbAuthTypes, keyValues)
		if err != nil {
			return nil, errors.Capture(err)
		}
		for _, info := range infos {
			result[corecredential.Key{
				Cloud: info.CloudName,
				Owner: owner,
				Name:  info.Label,
			}] = info
		}
	}
	if len(result) == 0 {
		return nil, errors.Errorf("cloud credentials for %q %w", owner, coreerrors.NotFound)
	}
	return result, errors.Capture(err)
}

// RemoveCloudCredential removes a cloud credential with the given name, cloud and owner..
func (st *State) RemoveCloudCredential(ctx context.Context, key corecredential.Key) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	credAttrDeleteStmt, err := st.Prepare(`
DELETE
FROM   cloud_credential_attribute
WHERE  cloud_credential_attribute.cloud_credential_uuid = $credentialUUID.uuid
`, credentialUUID{})
	if err != nil {
		return errors.Capture(err)
	}
	credDeleteStmt, err := st.Prepare(`
DELETE
FROM   cloud_credential
WHERE  cloud_credential.uuid = $credentialUUID.uuid
`, credentialUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	updateModelStmt, err := st.Prepare(`
UPDATE model
SET    cloud_credential_uuid = NULL
WHERE  cloud_credential_uuid = $modelCredentialUUID.cloud_credential_uuid
`, modelCredentialUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.credentialUUIDForKey(ctx, tx, key)
		if err != nil {
			return errors.Capture(err)
		}

		// Remove the credential from any models using it.
		modelCredUUID := modelCredentialUUID{UUID: uuid.String()}
		err = tx.Query(ctx, updateModelStmt, modelCredUUID).Run()
		if err != nil {
			return errors.Errorf("reseting model credentials: %w", err)
		}

		credUUID := credentialUUID{UUID: uuid.String()}
		if err := tx.Query(ctx, credAttrDeleteStmt, credUUID).Run(); err != nil {
			return errors.Errorf("deleting credential attributes: %w", err)
		}
		err = tx.Query(ctx, credDeleteStmt, credUUID).Run()
		if err != nil {
			return errors.Errorf("deleting credential: %w", err)
		}
		return nil
	})
}

// WatchCredential returns a new NotifyWatcher watching for changes to the specified credential.
func (st *State) WatchCredential(
	ctx context.Context,
	getWatcher func(
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error),
	key corecredential.Key,
) (watcher.NotifyWatcher, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var id corecredential.UUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		id, err = st.credentialUUIDForKey(ctx, tx, key)
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	result, err := getWatcher(
		eventsource.PredicateFilter("cloud_credential", changestream.All, eventsource.EqualsPredicate(id.String())),
	)
	if err != nil {
		return result, errors.Errorf("watching credential: %w", err)
	}
	return result, nil
}

// ModelsUsingCloudCredential returns a map of uuid->name for models which use the credential.
func (st *State) ModelsUsingCloudCredential(ctx context.Context, key corecredential.Key) (map[coremodel.UUID]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	credKey := credentialKey{
		CredentialName: key.Name,
		CloudName:      key.Cloud,
		OwnerName:      key.Owner.Name(),
	}

	query := `
SELECT m.* AS &modelNameAndUUID.*
FROM   v_model m
WHERE  m.cloud_credential_name = $credentialKey.name
AND    m.cloud_name = $credentialKey.cloud_name
AND    m.owner_name = $credentialKey.owner_name
`
	stmt, err := st.Prepare(query, credKey, modelNameAndUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[coremodel.UUID]string)
	var modelNameAndUUIDs []modelNameAndUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, credKey).GetAll(&modelNameAndUUIDs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	for _, m := range modelNameAndUUIDs {
		result[coremodel.UUID(m.UUID)] = m.Name
	}
	return result, nil
}
