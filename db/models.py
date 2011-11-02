from sqlalchemy import Column, Integer, String, ForeignKey
from sqlalchemy.orm import relationship, backref

from db.base import BaseModel


class Author(BaseModel):
    first_name = Column(String)
    middle_name = Column(String)
    last_name = Column(String)


class Book(BaseModel):
    path = Column(String)
    title = Column(String)

    author_id = Column(Integer, ForeignKey('author.id'))
    author = relationship("Author", backref=backref('books', order_by=id))
