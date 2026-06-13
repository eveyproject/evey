-- Drop the legacy 1-10 star-rating table. It was write-dead: nothing wrote
-- media_ratings (the only writer, CommandService.Rate, was never called from
-- any handler) and its single reader (GetRatingAggregate) always saw an empty
-- table. The live rating model is thumbs (like/dislike) in user_ratings. The
-- domain/rating package, app/rating command+query services and the postgres
-- RatingRepository were removed alongside this.
DROP TABLE IF EXISTS media_ratings;
