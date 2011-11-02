from sqlalchemy import Column, Integer, String, ForeignKey
from sqlalchemy.orm import relationship, backref

from db.base import BaseModel


class Author(BaseModel):
    __tablename__ = 'author'

    id = Column(Integer, primary_key=True)
    first_name = Column(String)
    middle_name = Column(String)
    last_name = Column(String)


class Book(BaseModel):
    __tablename__ = 'book'

    id = Column(Integer, primary_key=True)
    path = Column(String)
    title = Column(String)

    author_id = Column(Integer, ForeignKey('author.id'))
    author = relationship("Author", backref=backref('books', order_by=id))
