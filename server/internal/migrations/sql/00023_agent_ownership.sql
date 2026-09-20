-- +goose Up
-- Backstop legacy queue writers too: Conductor retains its reservation between
-- runs. Its own snapshots are inserted by the controller in the same transaction
-- that advances conductor_task. Ordinary AI requests cannot take this reservation.
-- +goose StatementBegin
CREATE TRIGGER protect_conductor_reservation
BEFORE INSERT ON agent_job
WHEN NEW.status IN ('pending','claimed','running','canceling')
 AND json_type(NEW.requestJson,'$.conductor') IS NULL
 AND EXISTS(SELECT 1 FROM conductor_task WHERE task=NEW.task AND state IN ('waiting','planning','queued','working'))
BEGIN
 SELECT RAISE(ABORT,'task is owned by Conductor');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER protect_conductor_reservation;
