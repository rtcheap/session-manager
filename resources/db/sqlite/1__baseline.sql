-- +migrate Up
CREATE TABLE `session` (
  `id` VARCHAR(50) NOT NULL,
  `status` VARCHAR(20) NOT NULL,
  `relay_server` VARCHAR(50) NOT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`)
);
CREATE TABLE `participant` (
  `id` VARCHAR(50) NOT NULL,
  `user_id` VARCHAR(50) NOT NULL,
  `session_id` VARCHAR(50) NOT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  FOREIGN KEY(`session_id`) REFERENCES `session`(`id`),
  UNIQUE(`session_id`, `user_id`),
  PRIMARY KEY (`id`)
);
-- +migrate Down
DROP TABLE IF EXISTS `participant`;
DROP TABLE IF EXISTS `session`;