package wikie

import (
	"encoding/json"
	"github.com/boltdb/bolt"
	"strings"
)

type AccessType int

const (
	PermissionRead AccessType = iota + 1
	PermissionWrite
)

type Permission struct {
	Path   string
	Access AccessType
}

type UserPermissions map[string][]Permission

func Init(db *bolt.DB, admins []string) error {
	return db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("perms"))
		if bucket == nil {
			var err error
			bucket, err = tx.CreateBucket([]byte("perms"))
			if err != nil {
				return err
			}
			perms := []Permission{
				{
					Path:   "/",
					Access: PermissionRead | PermissionWrite,
				},
			}
			b, err := json.Marshal(&perms)
			if err != nil {
				return err
			}
			for _, admin := range admins {
				return bucket.Put([]byte(admin), b)
			}
		}
		return nil
	})
}

func AddPermission(db *bolt.DB, user, path string, access AccessType) error {
	return db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("perms"))
		if v := bucket.Get([]byte(user)); v != nil {
			var perms []Permission
			err := json.Unmarshal(v, &perms)
			if err != nil {
				return err
			}
			perms = append(perms, Permission{
				Path:   path,
				Access: access,
			})
			b, err := json.Marshal(perms)
			if err != nil {
				return err
			}
			return bucket.Put([]byte(user), b)
		}
		b, err := json.Marshal([]Permission{{Path: path, Access: access}})
		if err != nil {
			return err
		}
		return bucket.Put([]byte(user), b)
	})
}

func RemovePermission(db *bolt.DB, user, path string, access AccessType) error {
	return db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("perms"))
		if v := bucket.Get([]byte(user)); v != nil {
			var perms []Permission
			err := json.Unmarshal(v, &perms)
			if err != nil {
				return err
			}
			for i, perm := range perms {
				if perm.Access == access && perm.Path == path {
					perms = append(perms[:i], perms[i+1:]...)
					b, err := json.Marshal(perms)
					if err != nil {
						return err
					}
					return bucket.Put([]byte(user), b)
				}
			}
		}
		return nil
	})
}

func HasPermission(db *bolt.DB, user, path string, access AccessType) (bool, error) {
	granted := false
	err := db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("perms"))
		if v := bucket.Get([]byte(user)); v != nil {
			var perms []Permission
			err := json.Unmarshal(v, &perms)
			if err != nil {
				return err
			}
			for _, perm := range perms {
				if perm.Access >= access && strings.Contains(path, perm.Path) {
					granted = true
					return nil
				}
			}
		}
		return nil
	})
	return granted, err
}

func GetPermissions(db *bolt.DB) (UserPermissions, error) {
	userPerms := make(UserPermissions)
	err := db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("perms"))
		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var perms []Permission
			err := json.Unmarshal(v, &perms)
			if err != nil {
				return err
			}
			userPerms[string(k)] = perms
		}
		return nil
	})
	return userPerms, err
}

func GetUserPermissions(db *bolt.DB, user string) (UserPermissions, error) {
	userPerms := make(UserPermissions)
	err := db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("perms"))
		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var perms []Permission
			err := json.Unmarshal(v, &perms)
			if err != nil {
				return err
			}
			if _, ok := userPerms[string(k)]; !ok {
				userPerms[string(k)] = []Permission{}
			}
			for _, perm := range perms {
				if ok, err := HasPermission(db, user, perm.Path, perm.Access); err == nil && ok {
					userPerms[string(k)] = append(userPerms[string(k)], perm)
				}
			}
		}
		return nil
	})
	return userPerms, err
}
