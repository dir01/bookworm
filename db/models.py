import sqlalchemy as sa
from sqlalchemy.orm import relationship
from sqlalchemy.schema import UniqueConstraint
from db.meta import Base


class Author(Base):
    __tablename__ = 'author'
    __table_args__ = (
        UniqueConstraint('first_name', 'middle_name', 'last_name'),
    )

    id = sa.Column(sa.Integer, primary_key=True)
    first_name = sa.Column(sa.String)
    middle_name = sa.Column(sa.String)
    last_name = sa.Column(sa.String)


class Genre(Base):
    __tablename__ = 'genre'

    id = sa.Column(sa.Integer, primary_key=True)
    name = sa.Column(sa.String, unique=True)


class Book(Base):
    __tablename__ = 'book'

    id = sa.Column(sa.Integer, primary_key=True)
    path = sa.Column(sa.String, unique=True)
    title = sa.Column(sa.String)
    year = sa.Column(sa.String)

    author_id = sa.Column(sa.Integer, sa.ForeignKey(Author.id))
    author = relationship(Author, backref='books')

    genre_id = sa.Column(sa.Integer, sa.ForeignKey(Genre.id))
    genre = relationship(Genre, backref='books')
