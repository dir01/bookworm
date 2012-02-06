import sqlalchemy as sa
from sqlalchemy.ext.declarative import declarative_base, declared_attr
from sqlalchemy.orm.exc import NoResultFound

__all__ = (
    'Base',
    'get_session'
)

class Base(object):
    @declared_attr
    def __tablename__(cls):
        return cls.__name__.lower()

    @declared_attr
    def id(cls):
        return sa.Column(sa.Integer, primary_key=True)

    @declared_attr
    def NoResultFound(cls):
        return NoResultFound

Base = declarative_base(cls=Base)


def get_session(url):
    from sqlalchemy.orm import sessionmaker, scoped_session
    from sqlalchemy import create_engine
    engine = create_engine(url)
    Session = scoped_session(sessionmaker(bind=engine))
    return Session()
