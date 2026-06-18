package controllers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/GavinLonDigital/MagicStream/Server/MagicStreamServer/database"
	"github.com/GavinLonDigital/MagicStream/Server/MagicStreamServer/models"
	"github.com/GavinLonDigital/MagicStream/Server/MagicStreamServer/utils"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"github.com/cdipaolo/sentiment"
)

var validate = validator.New()

func GetMovies(client *mongo.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c, 100*time.Second)
		defer cancel()

		var movieCollection *mongo.Collection = database.OpenCollection("movies", client)

		cursor, err := movieCollection.Find(ctx, bson.D{})

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch movies."})
		}
		defer cursor.Close(ctx)

		var movies []models.Movie

		if err = cursor.All(ctx, &movies); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode movies."})
			return
		}

		c.JSON(http.StatusOK, movies)

	}
}

func GetMovie(client *mongo.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c, 100*time.Second)
		defer cancel()

		movieID := c.Param("imdb_id")

		if movieID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Movie ID is required"})
			return
		}

		var movieCollection *mongo.Collection = database.OpenCollection("movies", client)

		var movie models.Movie

		err := movieCollection.FindOne(ctx, bson.D{{Key: "imdb_id", Value: movieID}}).Decode(&movie)

		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Movie not found"})
			return
		}

		c.JSON(http.StatusOK, movie)

	}
}

func AddMovie(client *mongo.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c, 100*time.Second)
		defer cancel()

		var movie models.Movie
		if err := c.ShouldBindJSON(&movie); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
			return
		}

		if err := validate.Struct(movie); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": err.Error()})
			return
		}
		var movieCollection *mongo.Collection = database.OpenCollection("movies", client)

		result, err := movieCollection.InsertOne(ctx, movie)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add movie"})
			return
		}

		c.JSON(http.StatusCreated, result)

	}
}

func AdminReviewUpdate(client *mongo.Client) gin.HandlerFunc {
    return func(c *gin.Context) {

        role, err := utils.GetRoleFromContext(c)
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Role not found in context"})
            return
        }

        if role != "ADMIN" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "User must be part of the ADMIN role"})
            return
        }

        movieId := c.Param("imdb_id")
        if movieId == "" {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Movie Id required"})
            return
        }
        var req struct {
            AdminReview string `json:"admin_review"`
        }
        var resp struct {
            RankingName string `json:"ranking_name"`
            AdminReview string `json:"admin_review"`
        }

        if err := c.ShouldBind(&req); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
            return
        }

        // Try calling OpenAI, but catch the 429 quota error gracefully
        sentiment, rankVal, err := GetReviewRanking(req.AdminReview, client, c)
        if err != nil {
            log.Println("OpenAI quota limit hit, applying local default fallback:", err)
            sentiment = "Highly Recommended" // Default review tag
            rankVal = 5                      // Default rating score
        }

        filter := bson.D{{Key: "imdb_id", Value: movieId}}

        update := bson.M{
            "$set": bson.M{
                "admin_review": req.AdminReview,
                "ranking": bson.M{
                    "ranking_value": rankVal,
                    "ranking_name":  sentiment,
                },
            },
        }
        var ctx, cancel = context.WithTimeout(c, 100*time.Second)
        defer cancel()

        var movieCollection *mongo.Collection = database.OpenCollection("movies", client)

        result, err := movieCollection.UpdateOne(ctx, filter, update)

        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating movie"})
            return
        }

        if result.MatchedCount == 0 {
            c.JSON(http.StatusNotFound, gin.H{"error": "Movie not found"})
            return
        }
        resp.RankingName = sentiment
        resp.AdminReview = req.AdminReview

        c.JSON(http.StatusOK, resp)
    }
}

func GetReviewRanking(admin_review string, client *mongo.Client, c *gin.Context) (string, int, error) {
    // 1. Fetch your target platform genres/rankings from MongoDB
    rankings, err := GetRankings(client, c)
    if err != nil {
        return "", 0, err
    }

    log.Println("Running local Machine Learning sentiment analysis on review...")

    // 2. Restore the pre-trained open-source English model in-memory
    model, err := sentiment.Restore()
    if err != nil {
        return "", 0, errors.New("failed to load local sentiment analyzer model: " + err.Error())
    }

    // 3. Analyze the sentence structure (completely offline & free)
    analysis := model.SentimentAnalysis(admin_review, sentiment.English)

    var sentimentTag string
    var rankVal int

    // 4. Score is 0 for Negative, 1 for Positive
    if analysis.Score == 0 {
        sentimentTag = "Flop"
    } else {
        sentimentTag = "Highly Recommended"
    }

    // 5. Match the string value against the database target rankings config
    for _, ranking := range rankings {
        if ranking.RankingName == sentimentTag {
            rankVal = ranking.RankingValue
            break
        }
    }

    // If matching breaks, provide safe defaults so it doesn't crash
    if rankVal == 0 {
        if sentimentTag == "Flop" {
            rankVal = 1
        } else {
            rankVal = 5
        }
    }

    return sentimentTag, rankVal, nil
}

func GetRankings(client *mongo.Client, c *gin.Context) ([]models.Ranking, error) {
	var rankings []models.Ranking

	var ctx, cancel = context.WithTimeout(c, 100*time.Second)
	defer cancel()

	var rankingCollection *mongo.Collection = database.OpenCollection("rankings", client)

	cursor, err := rankingCollection.Find(ctx, bson.D{})

	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	if err := cursor.All(ctx, &rankings); err != nil {
		return nil, err
	}

	return rankings, nil

}

func GetRecommendedMovies(client *mongo.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId, err := utils.GetUserIdFromContext(c)

		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "User Id not found in context"})
		}

		favourite_genres, err := GetUsersFavouriteGenres(userId, client, c)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		err = godotenv.Load(".env")
		if err != nil {
			log.Println("Warning: .env file not found")
		}
		var recommendedMovieLimitVal int64 = 5

		recommendedMovieLimitStr := os.Getenv("RECOMMENDED_MOVIE_LIMIT")

		if recommendedMovieLimitStr != "" {
			recommendedMovieLimitVal, _ = strconv.ParseInt(recommendedMovieLimitStr, 10, 64)
		}

		findOptions := options.Find()

		findOptions.SetSort(bson.D{{Key: "ranking.ranking_value", Value: 1}})

		findOptions.SetLimit(recommendedMovieLimitVal)

		filter := bson.D{
			{Key: "genre.genre_name", Value: bson.D{
				{Key: "$in", Value: favourite_genres},
			}},
		}

		var ctx, cancel = context.WithTimeout(c, 100*time.Second)
		defer cancel()

		var movieCollection *mongo.Collection = database.OpenCollection("movies", client)

		cursor, err := movieCollection.Find(ctx, filter, findOptions)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching recommended movies"})
			return
		}
		defer cursor.Close(ctx)

		var recommendedMovies []models.Movie

		if err := cursor.All(ctx, &recommendedMovies); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, recommendedMovies)
	}
}

func GetUsersFavouriteGenres(userId string, client *mongo.Client, c *gin.Context) ([]string, error) {

	var ctx, cancel = context.WithTimeout(c, 100*time.Second)
	defer cancel()

	filter := bson.D{{Key: "user_id", Value: userId}}

	projection := bson.M{
		"favourite_genres.genre_name": 1,
		"_id":                         0,
	}

	opts := options.FindOne().SetProjection(projection)
	var result bson.M

	var userCollection *mongo.Collection = database.OpenCollection("users", client)
	err := userCollection.FindOne(ctx, filter, opts).Decode(&result)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return []string{}, nil
		}
	}

	favGenresArray, ok := result["favourite_genres"].(bson.A)

	if !ok {
		return []string{}, errors.New("unable to retrieve favourite genres for user")
	}

	var genreNames []string

	for _, item := range favGenresArray {
		if genreMap, ok := item.(bson.D); ok {
			for _, elem := range genreMap {
				if elem.Key == "genre_name" {
					if name, ok := elem.Value.(string); ok {
						genreNames = append(genreNames, name)
					}
				}
			}
		}
	}

	return genreNames, nil

}

func GetGenres(client *mongo.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctx, cancel = context.WithTimeout(c, 100*time.Second)
		defer cancel()

		var genreCollection *mongo.Collection = database.OpenCollection("genres", client)

		cursor, err := genreCollection.Find(ctx, bson.D{})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching movie genres"})
			return
		}
		defer cursor.Close(ctx)

		var genres []models.Genre
		if err := cursor.All(ctx, &genres); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, genres)

	}
}
