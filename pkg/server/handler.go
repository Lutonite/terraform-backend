package server

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	"github.com/nimbolus/terraform-backend/pkg/auth"
	"github.com/nimbolus/terraform-backend/pkg/kms"
	"github.com/nimbolus/terraform-backend/pkg/lock"
	"github.com/nimbolus/terraform-backend/pkg/storage"
	"github.com/nimbolus/terraform-backend/pkg/terraform"
)

func HTTPResponse(w http.ResponseWriter, code int, body string) {
	log.Tracef("response: %d %s", code, body)
	w.WriteHeader(code)
	fmt.Fprint(w, body)
}

func StateHandler(store storage.Storage, locker lock.Locker, kms kms.KMS) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		defer req.Body.Close()
		if err != nil {
			HTTPResponse(w, http.StatusInternalServerError, err.Error())
			return
		}

		vars := mux.Vars(req)
		state := &terraform.State{
			ID:      terraform.GetStateID(vars["project"], vars["name"]),
			Project: vars["project"],
			Name:    vars["name"],
		}

		log.Infof("%s %s", req.Method, req.URL.Path)
		log.Tracef("request: %s %s: %s", req.Method, req.URL.Path, body)

		if ok, err := auth.Authenticate(req, state); err != nil {
			log.Warnf("failed process authentication for state id %s: %v", state.ID, err)
			HTTPResponse(w, http.StatusForbidden, err.Error())
			return
		} else if !ok {
			log.Warnf("failed to authenticate request for state id %s", state.ID)
			HTTPResponse(w, http.StatusForbidden, "Permission denied")
			return
		}

		switch req.Method {
		case "LOCK":
			log.Debugf("try to lock state with id %s", state.ID)
			state.Lock = body

			if ok, err := locker.Lock(state); err != nil {
				log.Errorf("failed to lock state with id %s: %v", state.ID, err)
				HTTPResponse(w, http.StatusInternalServerError, "")
			} else if !ok {
				log.Warnf("state with id %s is already locked by %s", state.ID, state.Lock)
				HTTPResponse(w, http.StatusLocked, string(state.Lock))
			} else {
				log.Debugf("state with id %s was locked successfully", state.ID)
				HTTPResponse(w, http.StatusOK, "")
			}
			return
		case "UNLOCK":
			log.Debugf("try to unlock state with id %s", state.ID)
			state.Lock = body

			if ok, err := locker.Unlock(state); err != nil {
				log.Errorf("failed to unlock state with id %s: %v", state.ID, err)
				HTTPResponse(w, http.StatusInternalServerError, "")
			} else if !ok {
				log.Warnf("failed to unlock state with id %s: %v", state.ID, err)
				HTTPResponse(w, http.StatusBadRequest, string(state.Lock))
			} else {
				log.Debugf("state with id %s was unlocked successfully", state.ID)
				HTTPResponse(w, http.StatusOK, "")
			}
			return
		case http.MethodGet:
			log.Debugf("get state with id %s", state.ID)
			stateID := state.ID
			state, err = store.GetState(state.ID)
			if err != nil {
				log.Warnf("failed to get state with id %s: %v", stateID, err)
				HTTPResponse(w, http.StatusBadRequest, err.Error())
				return
			}

			if kms != nil && len(state.Data) > 0 {
				state.Data, err = kms.Decrypt(state.Data)
				if err != nil {
					log.Errorf("failed to decrypt state with id %s: %v", state.ID, err)
					HTTPResponse(w, http.StatusInternalServerError, "")
					return
				}
			}

			HTTPResponse(w, http.StatusOK, string(state.Data))
			return
		case http.MethodPost:
			log.Debugf("save state with id %s", state.ID)

			state.Data, err = kms.Encrypt(body)
			if err != nil {
				log.Errorf("failed to encrypt state with id %s: %v", state.ID, err)
				HTTPResponse(w, http.StatusInternalServerError, "")
				return
			}

			err := store.SaveState(state)
			if err != nil {
				log.Warnf("failed to save state with id %s: %v", state.ID, err)
				HTTPResponse(w, http.StatusBadRequest, err.Error())
				return
			}

			HTTPResponse(w, http.StatusOK, "")
			return
		case http.MethodDelete:
			log.Debugf("delete state with id %s", state.ID)
			HTTPResponse(w, http.StatusNotImplemented, "Delete state is not implemented")
			return
		default:
			log.Warnf("unknown method %s called", req.Method)
			HTTPResponse(w, http.StatusNotImplemented, "Not implemented")
			return
		}
	}
}

func HealthHandler(w http.ResponseWriter, req *http.Request) {
	log.Debugf("%s %s", req.Method, req.URL.Path)
	HTTPResponse(w, http.StatusOK, "")
}