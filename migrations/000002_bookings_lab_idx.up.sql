-- Listing the students who booked a lab looks bookings up by lab_id (AC-14).
CREATE INDEX bookings_lab_id_idx ON bookings (lab_id);
