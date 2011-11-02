from sqlalchemy import create_engine
from sqlalchemy.ext.declarative import declarative_base, declared_attr
from sqlalchemy.orm import sessionmaker, scoped_session
from sqlalchemy.orm.exc import NoResultFound

engine = create_engine('sqlite:///:memory:', echo=True)
Session = scoped_session(sessionmaker(bind=engine))


class BaseModelCls(object):
    @declared_attr
    def __tablename__(cls):
        return cls.__name__.lower()

    @declared_attr
    def id(cls):
        return sa.Column(sa.Integer, primary_key=True)

    @declared_attr
    def NotFound(cls):
        return NoResultFound

    def save(self):
        Session().add(self)
        return self

    def delete(self):
        Session().delete(self)


BaseModel = declarative_base(
    cls=BaseModelCls,
    name='BaseModel',
    bind=engine
)
